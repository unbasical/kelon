package data

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"

	// Import mysql dirver
	_ "github.com/go-sql-driver/mysql"
	// import postgres driver
	_ "github.com/lib/pq"
)

type sqlDatastoreExecutor struct {
	dbPool  *sql.DB
	appConf *configs.AppConfig
}

func NewSQLDatastoreExecutor() data.DatastoreExecutor {
	return &sqlDatastoreExecutor{
		dbPool:  nil,
		appConf: nil,
	}
}

func (ds *sqlDatastoreExecutor) Configure(appConf *configs.AppConfig, alias string) error {
	// Validate config
	conf, e := extractAndValidateDatastore(appConf, alias)
	if e != nil {
		return errors.Wrap(e, "sqlDatastoreExecutor:")
	}

	// Init database connection pool
	db, err := sql.Open(conf.Type, getConnectionStringForPlatform(conf.Type, conf.Connection))
	if err != nil {
		return errors.Wrap(err, "SqlDatastore: Error while connecting to database")
	}

	// Configure metadata
	metadataError := ds.applyMetadataConfigs(conf, db)
	if metadataError != nil {
		return errors.Wrap(err, "sqlDatastoreExecutor: Error while configuring metadata")
	}

	// Ping database for 60 seconds every 3 seconds
	err = pingUntilReachable(alias, db.Ping)
	if err != nil {
		return errors.Wrap(err, "sqlDatastoreExecutor:")
	}

	ds.appConf = appConf
	ds.dbPool = db
	return nil
}

func (ds *sqlDatastoreExecutor) applyMetadataConfigs(conf *configs.Datastore, db *sql.DB) error {
	if conf.Metadata == nil {
		return nil
	}
	// Setup DatastoreTranslator
	if maxOpenValue, ok := conf.Metadata[constants.MetaMaxOpenConnections]; ok {
		maxOpen, err := strconv.Atoi(maxOpenValue)
		if err != nil {
			return errors.Wrap(err, "sqlDatastoreExecutor: Error while setting maxOpenConnections")
		}
		db.SetMaxOpenConns(maxOpen)
	}
	if maxIdleValue, ok := conf.Metadata[constants.MetaMaxIdleConnections]; ok {
		maxIdle, err := strconv.Atoi(maxIdleValue)
		if err != nil {
			return errors.Wrap(err, "sqlDatastoreExecutor: Error while setting maxIdleConnections")
		}
		db.SetMaxIdleConns(maxIdle)
	}
	if maxLifetimeSecondsValue, ok := conf.Metadata[constants.MetaConnectionMaxLifetimeSeconds]; ok {
		maxLifetimeSeconds, err := strconv.Atoi(maxLifetimeSecondsValue)
		if err != nil {
			return errors.Wrap(err, "sqlDatastoreExecutor: Error while setting connectionMaxLifetimeSeconds")
		}
		db.SetConnMaxLifetime(time.Second * time.Duration(maxLifetimeSeconds))
	}
	return nil
}

func (ds *sqlDatastoreExecutor) Execute(ctx context.Context, query data.DatastoreQuery) (bool, error) {
	sqlStatement, ok := query.Statement.(string)
	if !ok {
		return false, errors.Errorf("Passed statement was not of type string but of type: %T", query.Statement)
	}

	rows, err := ds.dbPool.Query(sqlStatement, query.Parameters...)
	if err != nil {
		return false, errors.Wrap(err, "sqlDatastoreExecutor: Error while executing statement")
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logging.LogForComponent("sqlDatastoreExecutor").Panic("Unable to close Result-Set!")
		}
	}()

	result := false
	for rows.Next() {
		var count int
		if err := rows.Scan(&count); err != nil {
			return false, errors.Wrap(err, "SqlDatastore: Unable to read result")
		}
		if count > 0 {
			logging.LogForComponent("sqlDatastoreExecutor").Debugf("Result row with count %d found! -> ALLOWED", count)
			result = true
			break
		}
	}

	if !result {
		logging.LogForComponent("sqlDatastoreExecutor").Debugf("No resulting row with count > 0 found! -> DENIED")
	}
	return result, nil
}
