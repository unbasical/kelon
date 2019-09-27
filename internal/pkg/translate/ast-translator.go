package translate

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/open-policy-agent/opa/rego"
)

type AstTranslatorConfig struct {
	Datastores map[string]*data.Datastore
}

type AstTranslator interface {
	Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error
	Process(response *rego.PartialQueries, datastore string) (bool, error)
}
