package data

import (
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

func MakeDatastores(config *configs.DatastoreConfig, dryRun bool) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		if dryRun {
			if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
				newDs := NewDatastore(NewSQLDatastoreTranslator(), NewLoggingDatastoreExecutor())
				logging.LogForComponent("factory").Infof("Init DryRun SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
				result[dsName] = &newDs
				continue
			}
			if ds.Type == data.TypeMongo {
				newDs := NewDatastore(NewMongoDatastoreTranslator(), NewLoggingDatastoreExecutor())
				logging.LogForComponent("factory").Infof("Init DryRun MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
				result[dsName] = &newDs
				continue
			}
		} else {
			if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
				newDs := NewDatastore(NewSQLDatastoreTranslator(), NewSQLDatastoreExecutor())
				logging.LogForComponent("factory").Infof("Init SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
				result[dsName] = &newDs
				continue
			}
			if ds.Type == data.TypeMongo {
				newDs := NewDatastore(NewMongoDatastoreTranslator(), NewMongoDatastoreExecuter())
				logging.LogForComponent("factory").Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
				result[dsName] = &newDs
				continue
			}
		}

		logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
	}
	return result
}
