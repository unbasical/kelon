package data

import (
	"errors"
	"github.com/Foundato/kelon/configs"
	"log"
)

type AbstractQuery struct {
}

type Datastore interface {
	Configure(appConf *configs.AppConfig) error
	Execute(query *AbstractQuery) (*interface{}, error)
}

type postgres struct {
	appConf    *configs.AppConfig
	configured bool
}

func NewPostgresDatastore() Datastore {
	return &postgres{
		appConf:    nil,
		configured: false,
	}
}

func (ds postgres) Configure(appConf *configs.AppConfig) error {
	if appConf == nil {
		return errors.New("PostgresDatastore: AppConfig not configured! ")
	}
	ds.appConf = appConf
	ds.configured = true
	log.Println("Configured PostgresDatastore")
	return nil
}

func (ds postgres) Execute(query *AbstractQuery) (*interface{}, error) {
	if !ds.configured {
		return nil, errors.New("PostgresDatastore was not configured! Please call Configure(). ")
	}

	// TODO implement Datastore access
	var stub interface{}
	stub = "I am a Stub!"
	return &stub, nil
}
