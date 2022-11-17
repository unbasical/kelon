package translate

import (
	"context"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/translate"
	"time"
)

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
func (trans astTranslator) Process(ctx context.Context, response *rego.PartialQueries, datastore string) (bool, error) {
	if !trans.configured {
		return false, errors.Errorf("AstTranslator was not configured! Please call Configure(). ")
	}

	preprocessedQueries, preprocessErr := newAstPreprocessor().Process(ctx, response.Queries, datastore)
	if preprocessErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during preprocessing.")
	}

	processedQuery, processErr := newAstProcessor(trans.config.SkipUnknown).Process(ctx, preprocessedQueries)
	if processErr != nil {
		return false, processErr
	}

	if targetDB, ok := trans.config.Datastores[datastore]; ok {
		pkg := ctx.Value(constants.LabelRegoPackage).(string)

		labels := map[string]string{
			constants.LabelRegoPackage: pkg,
			constants.LabelDBPoolName:  datastore,
		}

		function := func(ctx context.Context, args ...interface{}) (interface{}, error) {
			startTime := time.Now()
			decision, err := (*targetDB).Execute(ctx, processedQuery)
			duration := time.Since(startTime)

			// Update Metrics
			trans.appConf.MetricsProvider.UpdateHistogramMetric(ctx, constants.InstrumentDecisionDuration, duration.Milliseconds(), labels)
			return decision, err
		}

		val, err := trans.appConf.TraceProvider.ExecuteWithChildSpan(ctx, function, spanNameDatastoreQuery, labels)

		return val.(bool), err
	}
	return false, errors.Errorf("AstTranslator: Unable to find datastore: " + datastore)
}
