package data

import (
	"context"
	"encoding/json"
	"io"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

// loggingDatastoreExecutor implements the DatastoreExecutor interface by logging the data.DatastoreQuery
// instead of executing it against a database.
type loggingDatastoreExecutor struct {
	alias      string
	writer     io.Writer
	configured bool
	appConf    *configs.AppConfig
}

// NewLoggingDatastoreExecutor instantiates a new DatastoreExecutor, which logs the queries instead of
// executing it against a database.
func NewLoggingDatastoreExecutor(writer io.Writer) data.DatastoreExecutor {
	return &loggingDatastoreExecutor{
		writer:  writer,
		appConf: nil,
	}
}

// Configure -- see data.DatastoreExecutor
func (ds *loggingDatastoreExecutor) Configure(appConf *configs.AppConfig, alias string) error {
	if ds.configured {
		return nil
	}

	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true

	return nil
}

// Execute -- see data.DatastoreExecutor
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
