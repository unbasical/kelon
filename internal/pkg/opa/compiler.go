package opa

import (
	"errors"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/internal/pkg/translate"
	"net/http"
)

type CompilerConfig struct {
	Parser     request.RequestParser
	Translator translate.AstTranslator
	translate.AstTranslatorConfig
	request.RequestParserConfig
}

type PolicyCompiler interface {
	Configure(config *configs.AppConfig) PolicyCompiler
	Process(request *http.Request) (bool, error)
}

type policyCompiler struct {
	configured bool
	config     *configs.AppConfig
}

func NewPolicyCompiler() PolicyCompiler {
	return policyCompiler{
		configured: false,
		config:     nil,
	}
}

func (compiler policyCompiler) Configure(config *configs.AppConfig) PolicyCompiler {
	compiler.config = config
	compiler.configured = true
	return compiler
}

func (compiler policyCompiler) Process(request *http.Request) (bool, error) {
	if !compiler.configured {
		return false, errors.New("Compiler was not configured! Please call Configure(). ")
	}
	return true, nil
}
