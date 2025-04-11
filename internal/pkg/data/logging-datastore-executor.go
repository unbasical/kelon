package data

import (
	"context"
	"encoding/json"
	"io"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type loggingDatastoreExecutor struct {
	alias      string
	writer     io.Writer
	configured bool
	appConf    *configs.AppConfig
}

func NewLoggingDatastoreExecutor(writer io.Writer) data.DatastoreExecutor {
	return &loggingDatastoreExecutor{
		writer:  writer,
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

func (ds *loggingDatastoreExecutor) Execute(_ context.Context, query data.DatastoreQuery) (bool, error) {
	if ds.writer != nil {
		queryData := make(map[string]any)

		queryData["query"] = query.Statement
		queryData["parameter"] = query.Parameters

		jsonString, err := json.Marshal(queryData)
		if err != nil {
			return false, err
		}

		jsonString = append(jsonString, byte('\n'))
		_, err = ds.writer.Write(jsonString)
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
