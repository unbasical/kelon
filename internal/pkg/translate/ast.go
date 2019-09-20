package translate

import (
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	"log"
)

type AstTranslatorConfig struct {
	Datastore *data.Datastore
}

type AstTranslator interface {
	Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error
	Process(response *rego.PartialQueries) (*[]interface{}, error)
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
	if transConf.Datastore == nil {
		return errors.New("AstTranslator: Datastore not configured! ")
	}
	ds := *transConf.Datastore
	if err := ds.Configure(appConf); err != nil {
		return err
	}

	// Assign variables
	trans.appConf = appConf
	trans.config = transConf
	trans.preprocessor = &AstPreprocessor{}
	trans.processor = &AstProcessor{}
	trans.configured = true
	log.Println("Configured AstTranslator")
	return nil
}

func (trans astTranslator) Process(response *rego.PartialQueries) (*[]interface{}, error) {
	if !trans.configured {
		return nil, errors.New("AstTranslator was not configured! Please call Configure(). ")
	}

	tmpAst, preprocessErr := trans.preprocessor.Process(response.Queries)
	if preprocessErr != nil {
		return nil, errors.Wrap(preprocessErr, "AstTranslator: Error during preprocessing.")
	}
	processedQuery, processErr := trans.processor.Process(tmpAst)
	if processErr != nil {
		return nil, errors.Wrap(preprocessErr, "AstTranslator: Error during processing.")
	}

	return (*trans.config.Datastore).Execute(processedQuery)
}

func (trans astTranslator) Visit(v interface{}) ast.Visitor {
	fmt.Printf("Node: %+v\n", v)
	return trans
}
