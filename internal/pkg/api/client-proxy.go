package api

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/opa"
	"time"
)

type ClientProxyConfig struct {
	Compiler *opa.PolicyCompiler
	opa.PolicyCompilerConfig
}

type ClientProxy interface {
	Configure(appConf *configs.AppConfig, serverConf *ClientProxyConfig) error
	Start() error
	Stop(deadline time.Duration) error
}
