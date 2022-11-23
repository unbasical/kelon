package data

import (
	"context"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type loggingDatastoreExecutor struct {
	alias      string
	configured bool
	appConf    *configs.AppConfig
}

func NewLoggingDatastoreExecutor() data.DatastoreExecutor {
	return &loggingDatastoreExecutor{
		appConf: nil,
	}
}

func (ds *loggingDatastoreExecutor) Configure(appConf *configs.AppConfig, alias string) error {
	if ds.configured {
		return nil
	}

	ds.appConf = appConf

	ds.alias = alias

	ds.configured = true
	return nil
}

func (ds *loggingDatastoreExecutor) Execute(ctx context.Context, query data.DatastoreQuery) (bool, error) {
	logging.LogForComponent("outputDatastoreExecutor").
		WithField("statement", query.Statement).
		WithField("parameters", query.Parameters).
		Infof("Writing Query:")
	return true, nil
}
