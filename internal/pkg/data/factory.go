package data

import (
	"io"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

func MakeDatastores(config *configs.ExternalConfig, dsLoggingWriter io.Writer, loggingMode bool) map[string]*data.Datastore {
	if loggingMode {
		return makeLoggingDatastores(config, dsLoggingWriter)
	}
	return makeExecutingDatastores(config)
}

func makeExecutingDatastores(config *configs.ExternalConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		switch {
		case ds.Type == data.TypeMysql || ds.Type == data.TypePostgres:
			newDs := NewDatastore(NewSQLDatastoreTranslator(), NewSQLDatastoreExecutor())
			logging.LogForComponent("factory").Infof("Init SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		case ds.Type == data.TypeMongo:
			newDs := NewDatastore(NewMongoDatastoreTranslator(), NewMongoDatastoreExecuter())
			logging.LogForComponent("factory").Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		default:
			logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
		}
	}
	return result
}

func makeLoggingDatastores(config *configs.ExternalConfig, dsLoggingWriter io.Writer) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	for dsName, ds := range config.Datastores {
		switch {
		case ds.Type == data.TypeMysql || ds.Type == data.TypePostgres:
			newDs := NewDatastore(NewSQLDatastoreTranslator(), NewSQLDatastoreExecutor())
			logging.LogForComponent("factory").Infof("Init DryRun SqlDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		case ds.Type == data.TypeMongo:
			newDs := NewDatastore(NewMongoDatastoreTranslator(), NewLoggingDatastoreExecutor(dsLoggingWriter))
			logging.LogForComponent("factory").Infof("Init DryRun MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		default:
			logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q! Type is not supported yet!", ds.Type)
		}
	}
	return result
}
