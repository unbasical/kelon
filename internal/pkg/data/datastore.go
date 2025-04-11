package data

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

const keyHost = "host"
const keyPort = "port"
const keyDB = "database"
const keyUser = "user"
const keyPassword = "password"

// extractAndValidateDatastore tries to extract the datastore config via the provided alias
// and validates the connection configuration for missing attributes
func extractAndValidateDatastore(appConf *configs.AppConfig, alias string) (*configs.Datastore, error) {
	if appConf == nil {
		return nil, errors.Errorf("AppConfig not configured!")
	}
	if alias == "" {
		return nil, errors.Errorf("Empty alias provided!")
	}
	// Validate configuration
	conf, ok := appConf.Datastores[alias]
	if !ok {
		return nil, errors.Errorf("No datastore with alias [%s] configured!", alias)
	}
	if strings.EqualFold(conf.Type, "") {
		return nil, errors.Errorf("Alias of datastore is empty! Must be one of %+v!", sql.Drivers())
	}
	if err := validateConnection(alias, conf.Connection); err != nil {
		return nil, err
	}

	return conf, nil
}

// pingUntilReachable tries to call the provided ping function until a stable connection is established
func pingUntilReachable(alias string, ping func() error) error {
	var pingFailure error
	for i := 0; i < 20; i++ {
		if pingFailure = ping(); pingFailure == nil {
			// Ping succeeded
			return nil
		}
		logging.LogForComponent("datastore").Infof("Waiting for [%s] to be reachable...", alias)
		<-time.After(3 * time.Second)
	}
	if pingFailure != nil {
		return errors.Wrap(pingFailure, "Unable to ping database")
	}
	return nil
}

// validateConnection checks whether all necessary config options are provided
func validateConnection(alias string, conn map[string]string) error {
	if _, ok := conn[keyHost]; !ok {
		return errors.Errorf("SqlDatastore: Field %s is missing in configured connection with alias %s!", keyHost, alias)
	}
	if _, ok := conn[keyPort]; !ok {
		return errors.Errorf("SqlDatastore: Field %s is missing in configured connection with alias %s!", keyPort, alias)
	}
	if _, ok := conn[keyDB]; !ok {
		return errors.Errorf("SqlDatastore: Field %s is missing in configured connection with alias %s!", keyDB, alias)
	}
	if _, ok := conn[keyUser]; !ok {
		return errors.Errorf("SqlDatastore: Field %s is missing in configured connection with alias %s!", keyUser, alias)
	}
	if _, ok := conn[keyPassword]; !ok {
		return errors.Errorf("SqlDatastore: Field %s is missing in configured connection with alias %s!", keyPassword, alias)
	}
	return nil
}

// getConnectionStringForPlatform builds a connection string from the connection options for a specific platform
func getConnectionStringForPlatform(platform string, conn map[string]string) string {
	params := extractAndSortConnectionParameters(conn)

	switch platform {
	case data.TypePostgres:
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s%s", params.host, params.port, params.user, params.password, params.dbname, createConnOptionsString(params.options, " ", " "))
	case data.TypeMysql:
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", params.user, params.password, params.host, params.port, params.dbname, createConnOptionsString(params.options, "&", "?"))
	case data.TypeMongo:
		return fmt.Sprintf("mongodb://%s:%s@%s:%s/%s%s", params.user, params.password, params.host, params.port, params.dbname, createConnOptionsString(params.options, "&", "?"))
	default:
		logging.LogForComponent("datastore").Panic(fmt.Sprintf("Platform [%s] is not a supported DatastoreTranslator!", platform))
		return ""
	}
}

// getPreparePlaceholderForPlatform returns the platform specific placeholder for prepared statements
func getPreparePlaceholderForPlatform(platform string, argCounter int) string {
	switch platform {
	case data.TypePostgres:
		return fmt.Sprintf("$%d", argCounter)
	case data.TypeMysql:
		return "?"
	default:
		logging.LogForComponent("datastore").Panic(fmt.Sprintf("Platform [%s] is not a supported for prepared statements!", platform))
		return ""
	}
}

type connParams struct {
	host     string
	port     string
	user     string
	password string
	dbname   string
	options  []string
}

// Extract and sort all connection parameters by importance.
// Each option has the format <key>=<value>
func extractAndSortConnectionParameters(conn map[string]string) connParams {
	params := connParams{}

	for key, value := range conn {
		switch key {
		case keyHost:
			params.host = value
		case keyPort:
			params.port = value
		case keyUser:
			params.user = value
		case keyPassword:
			params.password = value
		case keyDB:
			params.dbname = value
		default:
			params.options = append(params.options, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return params
}

// createConnOptionsString formats additional options for the database connection string
func createConnOptionsString(options []string, delimiter, prefix string) string {
	optionString := strings.Join(options, delimiter)
	if len(options) > 0 {
		optionString = prefix + optionString
	}
	return optionString
}
