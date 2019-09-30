package data

import (
	"github.com/Foundato/kelon/configs"
	log "github.com/sirupsen/logrus"
)

func MakeDatastores(config *configs.DatastoreConfig) map[string]*Datastore {
	result := make(map[string]*Datastore)
	for dsName, ds := range config.Datastores {
		newDs := NewSqlDatastore()
		log.Infof("Init SqlDatastore of type [%s] with alias [%s]\n", ds.Type, dsName)
		result[dsName] = &newDs
	}
	return result
}
