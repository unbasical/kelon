package translate

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/translate"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
		return errors.New("AstTranslator: Datastores not configured! ")
	}
	if len(transConf.Datastores) == 0 {
		return errors.New("AstTranslator: At least one datastore is needed! ")
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
	log.Infoln("Configured AstTranslator")
	return nil
}

// See translate.AstTranslator.
func (trans astTranslator) Process(response *rego.PartialQueries, datastore string, queryContext interface{}) (bool, error) {
	if !trans.configured {
		return false, errors.New("AstTranslator was not configured! Please call Configure(). ")
	}

	preprocessedQueries, preprocessErr := newAstPreprocessor().Process(response.Queries, datastore)
	if preprocessErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during preprocessing.")
	}

	processedQuery, processErr := newAstProcessor().Process(preprocessedQueries)
	if processErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during processing.")
	}

	if targetDb, ok := trans.config.Datastores[datastore]; ok {
		return (*targetDb).Execute(processedQuery, queryContext)
	}
	return false, errors.New("AstTranslator: Unable to find datastore: " + datastore)
}
