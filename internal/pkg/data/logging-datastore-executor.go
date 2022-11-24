package data

import (
	"context"
	"encoding/json"
	"os"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type loggingDatastoreExecutor struct {
	alias      string
	file       *os.File
	configured bool
	appConf    *configs.AppConfig
}

func NewLoggingDatastoreExecutor(file *os.File) data.DatastoreExecutor {
	return &loggingDatastoreExecutor{
		file:    file,
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
	if ds.file != nil {
		queryData := make(map[string]interface{})

		queryData["query"] = query.Statement
		queryData["parameter"] = query.Parameters

		jsonString, err := json.Marshal(queryData)
		if err != nil {
			return false, err
		}

		_, err = (*ds.file).Write(jsonString)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	logging.LogForComponent("loggingDatastoreExecutor").
		WithField("statement", query.Statement).
		WithField("parameters", query.Parameters).
		Infof("Logging Query:")
	return true, nil
}
