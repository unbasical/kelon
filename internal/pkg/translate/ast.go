package translate

import (
	"errors"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/data"
	"log"
)

type AstTranslatorConfig struct {
	Datastore *data.Datastore
}

type AstTranslator interface {
	Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error
	Process(ast *interface{}) (*interface{}, error)
}

type astTranslator struct {
	appConf    *configs.AppConfig
	config     *AstTranslatorConfig
	configured bool
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
	trans.configured = true
	log.Println("Configured AstTranslator")
	return nil
}

func (trans astTranslator) Process(ast *interface{}) (*interface{}, error) {
	if !trans.configured {
		return nil, errors.New("AstTranslator was not configured! Please call Configure(). ")
	}

	// TODO implement AST-translation
	var stub interface{}
	stub = "I am a Stub!"
	return &stub, nil
}
