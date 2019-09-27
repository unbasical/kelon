package request

import (
	"github.com/Foundato/kelon/configs"
)

type PathProcessorConfig struct {
	PathMapper *PathMapper
}

type PathProcessorOutput struct {
	Datastore string
	Package   string
	Path      []string
	Queries   map[string]interface{}
}

type PathProcessor interface {
	Configure(appConf *configs.AppConfig, processorConf *PathProcessorConfig) error
	Process(input interface{}) (*PathProcessorOutput, error)
}
