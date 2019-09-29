package opa

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/internal/pkg/translate"
	"net/http"
)

type PolicyCompilerConfig struct {
	OpaConfigPath *string
	RegoDir       *string
	Prefix        *string
	PathProcessor *request.PathProcessor
	Translator    *translate.AstTranslator
	translate.AstTranslatorConfig
	request.PathProcessorConfig
}

type PolicyCompiler interface {
	Configure(appConfig *configs.AppConfig, compConfig *PolicyCompilerConfig) error
	Process(request *http.Request) (bool, error)
}
