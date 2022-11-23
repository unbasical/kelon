package data

import (
	"context"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type defaultDatastore struct {
	appConf    *configs.AppConfig
	alias      string
	configured bool
	translator data.DatastoreTranslator
	executor   data.DatastoreExecutor
}

func NewDatastore(translator data.DatastoreTranslator, executor data.DatastoreExecutor) data.Datastore {
	return &defaultDatastore{
		appConf:    nil,
		alias:      "",
		configured: false,
		translator: translator,
		executor:   executor,
	}
}

func (ds *defaultDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Configure translator
	if ds.translator == nil {
		return errors.Errorf("Datastore: DatastoreTranslator not configured!")
	}
	if err := ds.translator.Configure(appConf, alias); err != nil {
		return errors.Wrap(err, "Datastore: Error while configuring datastore translator")
	}

	// Configure executor
	if ds.executor == nil {
		return errors.Errorf("Datastore: DatastoreExecutor not configured!")
	}
	if err := ds.executor.Configure(appConf, alias); err != nil {
		return errors.Wrap(err, "Datastore: Error while configuring datastore executor")
	}

	// Assign values
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	logging.LogForComponent("Datastore").Infof("Configured [%s]", alias)
	return nil
}

func (ds *defaultDatastore) Execute(ctx context.Context, astQuery data.Node) (bool, error) {
	if !ds.configured {
		return false, errors.Errorf("Datastore: Datastore was not configured! Please call Configure().")
	}

	// Translate Query-AST to native Query
	dsQuery, err := ds.translator.Execute(ctx, astQuery)
	if err != nil {
		return false, err
	}

	// Execute native Query
	return ds.executor.Execute(ctx, dsQuery)
}
