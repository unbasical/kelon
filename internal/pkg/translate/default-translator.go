package translate

import (
	"context"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/telemetry"
	"github.com/unbasical/kelon/pkg/translate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"time"
)

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

		// Create Span
		if trans.appConf.TraceProvider != nil {
			var s interface{}
			ctx, s = trans.appConf.TraceProvider.StartChildSpan(ctx, "datastore.query")

			span := s.(trace.Span)
			defer span.End()

			attr := []attribute.KeyValue{
				attribute.Key(constants.LabelRegoPackage).String(pkg),
				attribute.Key(constants.LabelDBPoolName).String(datastore),
			}
			span.SetAttributes(attr...)
		}

		startTime := time.Now()
		decision, err := (*targetDB).Execute(ctx, processedQuery)
		duration := time.Since(startTime)

		// Record Error
		if err != nil && trans.appConf.TraceProvider != nil {
			trans.appConf.TraceProvider.RecordError(ctx, err)
		}

		// Update Metrics
		if trans.appConf.MetricsProvider != nil {
			trans.appConf.MetricsProvider.WriteMetricQuery(ctx, telemetry.DbQuery{Duration: duration.Milliseconds(), Package: pkg, PoolName: datastore})
		}
		return decision, err
	}
	return false, errors.Errorf("AstTranslator: Unable to find datastore: " + datastore)
}
