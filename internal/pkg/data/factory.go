package data

import (
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

func MakeDatastores(config *configs.DatastoreConfig) map[string]*data.DatastoreTranslator {
	result := make(map[string]*data.DatastoreTranslator)
	for dsName, ds := range config.Datastores {
		if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
			newDs := NewSQLDatastore(NewSQLDatastoreExecutor())
			logging.LogForComponent("factory").Infof("Init SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}
		if ds.Type == data.TypeMongo {
			newDs := NewMongoDatastore(NewMongoDatastoreExecuter())
			logging.LogForComponent("factory").Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}

		logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
	}
	return result
}
