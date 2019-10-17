package data

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/data"
	log "github.com/sirupsen/logrus"
)

func MakeDatastores(config *configs.DatastoreConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		newDs := NewSqlDatastore()
		log.Infof("Init SqlDatastore of type [%s] with alias [%s]\n", ds.Type, dsName)
		result[dsName] = &newDs
	}
	return result
}
