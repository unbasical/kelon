package translate

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type AstTranslatorConfig struct {
	Datastores map[string]*data.Datastore
}

type AstTranslator interface {
	Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error
	Process(response *rego.PartialQueries, datastore string) (bool, error)
}

type astTranslator struct {
	appConf      *configs.AppConfig
	config       *AstTranslatorConfig
	preprocessor *AstPreprocessor
	processor    *AstProcessor
	configured   bool
}

func NewAstTranslator() AstTranslator {
	return &astTranslator{
		appConf:    nil,
		config:     nil,
		configured: false,
	}
}

func (trans *astTranslator) Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error {
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
	trans.preprocessor = &AstPreprocessor{}
	trans.processor = &AstProcessor{}
	trans.configured = true
	log.Infoln("Configured AstTranslator")
	return nil
}

func (trans astTranslator) Process(response *rego.PartialQueries, datastore string) (bool, error) {
	if !trans.configured {
		return false, errors.New("AstTranslator was not configured! Please call Configure(). ")
	}

	preprocessedQueries, preprocessErr := trans.preprocessor.Process(response.Queries, datastore)
	if preprocessErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during preprocessing.")
	}

	processedQuery, processErr := trans.processor.Process(preprocessedQueries)
	if processErr != nil {
		return false, errors.Wrap(preprocessErr, "AstTranslator: Error during processing.")
	}

	if targetDb, ok := trans.config.Datastores[datastore]; ok {
		return (*targetDb).Execute(processedQuery)
	} else {
		return false, errors.New("AstTranslator: Unable to find datastore: " + datastore)
	}
}

func (trans astTranslator) Visit(v interface{}) ast.Visitor {
	log.Debugf("Node: %+v\n", v)
	return trans
}