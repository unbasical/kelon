package opa

import (
	"errors"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/internal/pkg/translate"
	"log"
	"net/http"
)

type CompilerConfig struct {
	PathProcessor *request.PathProcessor
	Translator    *translate.AstTranslator
	translate.AstTranslatorConfig
	request.PathProcessorConfig
}

type PolicyCompiler interface {
	Configure(appConfig *configs.AppConfig, compConfig *CompilerConfig) error
	Process(request *http.Request) (bool, error)
}

type policyCompiler struct {
	configured bool
	appConfig  *configs.AppConfig
	config     *CompilerConfig
}

func NewPolicyCompiler() PolicyCompiler {
	return &policyCompiler{
		configured: false,
		config:     nil,
	}
}

func (compiler *policyCompiler) Configure(appConf *configs.AppConfig, compConf *CompilerConfig) error {
	// Configure PathProcessor
	if compConf.PathProcessor == nil {
		return errors.New("PolicyCompiler: PathProcessor not configured! ")
	}
	parser := *compConf.PathProcessor
	if err := parser.Configure(appConf, &compConf.PathProcessorConfig); err != nil {
		return err
	}

	// Configure AstTranslator
	if compConf.Translator == nil {
		return errors.New("PolicyCompiler: Translator not configured! ")
	}
	translator := *compConf.Translator
	if err := translator.Configure(appConf, &compConf.AstTranslatorConfig); err != nil {
		return err
	}

	// Assign variables
	compiler.appConfig = appConf
	compiler.config = compConf
	compiler.configured = true
	log.Println("Configured PolicyCompiler")
	return nil
}

func (compiler policyCompiler) Process(request *http.Request) (bool, error) {
	if !compiler.configured {
		return false, errors.New("PolicyCompiler was not configured! Please call Configure(). ")
	}

	processor := *compiler.config.PathProcessor
	mappedPath, datastore, err := processor.Process(request)
	if err != nil {
		log.Println("PolicyCompiler: Error during path processing: " + err.Error())
		return false, nil
	}

	fmt.Printf("%s -> %v\n", datastore, mappedPath)
	// TODO implement OPA-compiler
	return true, nil
}
