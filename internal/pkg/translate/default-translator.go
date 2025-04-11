package translate

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"os"
	"time"

	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"github.com/unbasical/kelon/pkg/translate"
)

const EnvHugeQueryPrint = "HUGE_QUERY_PRINT"
const spanNameDatastoreQuery string = "datastore.query"

type astTranslator struct {
	appConf    *configs.AppConfig
	config     *translate.AstTranslatorConfig
	configured bool
}

// Create a new instance of the default translate.AstTranslator.
func NewAstTranslator() translate.AstTranslator {
	return &astTranslator{
		appConf:    nil,
		config:     nil,
		configured: false,
	}
}

// See translate.AstTranslator.
func (trans *astTranslator) Configure(appConf *configs.AppConfig, transConf *translate.AstTranslatorConfig) error {
	// Exit if already configured
	if trans.configured {
		return nil
	}

	// Configure subcomponents
	if transConf.Datastores == nil {
		return errors.Errorf("AstTranslator: Datastores not configured! ")
	}
	if len(transConf.Datastores) == 0 {
		return errors.Errorf("AstTranslator: At least one datastore is needed! ")
	}
	for dsName, ds := range transConf.Datastores {
		if err := (*ds).Configure(appConf, dsName); err != nil {
			return errors.Wrap(err, "AstTranslator: Error while configuring datastore "+dsName)
		}
	}

	// Assign variables
	trans.appConf = appConf
	trans.config = transConf
	trans.configured = true
	logging.LogForComponent("astTranslator").Infoln("Configured")
	return nil
}

// See translate.AstTranslator.
func (trans *astTranslator) Process(ctx context.Context, response *rego.PartialQueries, datastores []string) (bool, error) {
	if !trans.configured {
		return false, errors.Errorf("AstTranslator was not configured! Please call Configure(). ")
	}

	debugCtx := map[string]interface{}{}
	debugCtx["regoQueries"] = fmt.Sprintf("%v", response.Queries)

	preprocessedQueries, preprocessErr := newAstPreprocessor().Process(ctx, response.Queries, datastores)
	if preprocessErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during preprocessing.")
	}
	preproQueries, _ := json.Marshal(preprocessedQueries)
	debugCtx["preprocessedQueries"] = fmt.Sprintf("%v", preproQueries)

	datastoreSpecificQueries := make(map[string]data.Node)
	for _, preprocessed := range preprocessedQueries {
		processedQuery, processErr := newAstProcessor(trans.config.SkipUnknown, trans.config.ValidateMode).Process(ctx, preprocessed.query)
		if processErr != nil {
			return false, processErr
		}

		node, ok := datastoreSpecificQueries[preprocessed.datastore]
		if !ok {
			node = data.Union{Clauses: []data.Node{}}
		}
		union, _ := node.(data.Union)

		datastoreSpecificQueries[preprocessed.datastore] = data.Union{Clauses: append(union.Clauses, processedQuery)}
	}
	debugCtx["datastoreSpecificQueries"] = datastoreSpecificQueries

	for datastore, specificQuery := range datastoreSpecificQueries {
		queryToExecute := specificQuery
		if targetDB, ok := trans.config.Datastores[datastore]; ok {
			pkg := ctx.Value(constants.ContextKeyRegoPackage).(string)

			labels := map[string]string{
				constants.LabelRegoPackage: pkg,
				constants.LabelDBPoolName:  datastore,
			}

			function := func(ctx context.Context, args ...interface{}) (interface{}, error) {
				startTime := time.Now()
				decision, err := (*targetDB).Execute(ctx, queryToExecute)
				duration := time.Since(startTime)
				if err != nil {
					var qle data.QueryLengthError
					if stdErrors.As(err, &qle) {
						debugCtx["datastoreQuery"] = fmt.Sprintf("%v", qle.Query)

						if os.Getenv(EnvHugeQueryPrint) != "" {
							logging.LogForComponent("QUERY_DEBUG").WithFields(debugCtx).Error("Huge Query detected!")
						}
						err = nil
					}
				}

				// Update Metrics
				trans.appConf.MetricsProvider.UpdateHistogramMetric(ctx, constants.InstrumentDecisionDuration, duration.Milliseconds(), labels)
				return decision, err
			}

			res, err := trans.appConf.TraceProvider.ExecuteWithChildSpan(ctx, function, spanNameDatastoreQuery, labels)
			if err != nil {
				return false, err
			}

			if res.(bool) {
				return true, nil
			}
		} else {
			return false, errors.Errorf("AstTranslator: Unable to find datastore: %s", datastore)
		}
	}
	return false, nil
}
