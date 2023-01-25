package data

import (
	"context"
	"io"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"github.com/unbasical/kelon/pkg/extensions"
)

func MakeDatastores(ctx context.Context, config *configs.ExternalConfig, extensionFactory extensions.Factory, dsLoggingWriter io.Writer, loggingMode bool) map[string]*data.Datastore {
	if loggingMode {
		return makeLoggingDatastores(ctx, config, extensionFactory, dsLoggingWriter)
	}
	return makeExecutingDatastores(ctx, config, extensionFactory)
}

func makeExecutingDatastores(ctx context.Context, config *configs.ExternalConfig, extensionFactory extensions.Factory) map[string]*data.Datastore {
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
			newDs, err := extensionFactory.MakeDatastore(ctx, ds.Type)
			if err != nil {
				logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q: %s", ds.Type, err.Error())
			}

			logging.LogForComponent("factory").Infof("Init datastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		}
	}
	return result
}

func makeLoggingDatastores(ctx context.Context, config *configs.ExternalConfig, extensionFactory extensions.Factory, dsLoggingWriter io.Writer) map[string]*data.Datastore {
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
			newDs, err := extensionFactory.MakeDatastore(ctx, ds.Type)
			if err != nil {
				logging.LogForComponent("factory").Fatalf("Unable to init datastore of type %q: %s", ds.Type, err.Error())
			}

			logging.LogForComponent("factory").Infof("Init datastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		}
	}
	return result
}
