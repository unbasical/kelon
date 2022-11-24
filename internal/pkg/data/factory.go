package data

import (
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

func MakeDatastores(config *configs.DatastoreConfig, loggingMode bool) map[string]*data.Datastore {
	if loggingMode {
		return makeLoggingDatastores(config)
	}
	return makeExecutingDatastores(config)
}

func makeExecutingDatastores(config *configs.DatastoreConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
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

		logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
	}
	return result
}

func makeLoggingDatastores(config *configs.DatastoreConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
			newDs := NewDatastore(NewSQLDatastoreTranslator(), NewLoggingDatastoreExecutor(config.OutputFile))
			logging.LogForComponent("factory").Infof("Init DryRun SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}
		if ds.Type == data.TypeMongo {
			newDs := NewDatastore(NewMongoDatastoreTranslator(), NewLoggingDatastoreExecutor(config.OutputFile))
			logging.LogForComponent("factory").Infof("Init DryRun MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}

		logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
	}
	return result
}
