package request

import (
	"errors"
	"github.com/Foundato/kelon/configs"
	"log"
)

type PathMapper interface {
	Configure(appConf *configs.AppConfig) error
	Map(path []string) ([]string, error)
}

type pathMapper struct {
	appConf    *configs.AppConfig
	configured bool
}

func NewPathMapper() PathMapper {
	return &pathMapper{
		appConf:    nil,
		configured: false,
	}
}

func (mapper *pathMapper) Configure(appConf *configs.AppConfig) error {
	if appConf == nil {
		return errors.New("PathMapper: AppConfig not configured! ")
	}
	mapper.appConf = appConf
	mapper.configured = true
	log.Println("Configured PathMapper")
	return nil
}

func (mapper pathMapper) Map(path []string) ([]string, error) {
	if !mapper.configured {
		return nil, errors.New("PathMapper was not configured! Please call Configure(). ")
	}

	// TODO implement Path-mapping
	return []string{}, nil
}
