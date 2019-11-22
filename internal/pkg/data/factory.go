package data

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/data"
	log "github.com/sirupsen/logrus"
)

func MakeDatastores(config *configs.DatastoreConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
			newDs := NewSQLDatastore()
			log.Infof("Init SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}
		if ds.Type == data.TypeMongo {
			newDs := NewMongoDatastore()
			log.Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}

		log.Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
	}
	return result
}
