package data

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

//nolint:gochecknoglobals,gocritic
var (
	keyHost     = "host"
	keyPort     = "port"
	keyDB       = "database"
	keyUser     = "user"
	keyPassword = "password"
	keyInsecure = "insecure"
	keyToken    = "token"
)

func extractAndValidateDatastore(appConf *configs.AppConfig, alias string) (*configs.Datastore, error) {
	if appConf == nil {
		return nil, errors.Errorf("AppConfig not configured!")
	}
	if alias == "" {
		return nil, errors.Errorf("Empty alias provided!")
	}
	// Validate configuration
	conf, ok := appConf.Data.Datastores[alias]
	if !ok {
		return nil, errors.Errorf("no datastore with alias [%s] configured!", alias)
	}
	if strings.EqualFold(conf.Type, "") {
		return nil, errors.Errorf("alias of datastore is empty! Must be one of %+v!", sql.Drivers())
	}
	if err := validateConnection(alias, conf.Connection); err != nil {
		return nil, err
	}

	conf.CallOperandsDir = appConf.Data.CallOperandsDir
	return conf, nil
}

func extractAndValidateSpiceDBDatastore(appConf *configs.AppConfig, alias string) (*configs.SpiceDB, error) {
	if appConf == nil {
		return nil, errors.Errorf("AppConfig not configured!")
	}
	if alias == "" {
		return nil, errors.Errorf("Empty alias provided!")
	}

	// Validate configuration
	conf, ok := appConf.Data.Datastores[alias]
	if !ok {
		return nil, errors.Errorf("no datastore with alias [%s] configured!", alias)
	}
	if strings.EqualFold(conf.Type, "") {
		return nil, errors.Errorf("alias of datastore is empty! Must be one of %+v!", sql.Drivers())
	}
	if err := validateSpiceDBConnection(alias, conf.Connection); err != nil {
		return nil, err
	}

	// Build SpiceDB Config
	var spiceConf configs.SpiceDB
	spiceConf.Token = conf.Connection[keyToken]
	spiceConf.Insecure, _ = strconv.ParseBool(conf.Connection[keyInsecure])
	spiceConf.Endpoint = fmt.Sprintf("%s:%s", conf.Connection[keyHost], conf.Connection[keyPort])

	return &spiceConf, nil
}

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

func loadCallOperands(conf *configs.Datastore) (map[string]func(args ...string) (string, error), error) {
	callOpsFile := fmt.Sprintf("%s/%s.yml", conf.CallOperandsDir, strings.ToLower(conf.Type))
	handlers, err := LoadDatastoreCallOpsFile(callOpsFile)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to load call operands as handlers")
	}

	operands := map[string]func(args ...string) (string, error){}
	for _, handler := range handlers {
		operands[handler.Handles()] = handler.Map
	}
	return operands, nil
}

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

func validateSpiceDBConnection(alias string, conn map[string]string) error {
	if _, ok := conn[keyHost]; !ok {
		return errors.Errorf("SpiceDB: Field %s is missing in configured connection with alias %s!", keyHost, alias)
	}
	if _, ok := conn[keyPort]; !ok {
		return errors.Errorf("SpiceDB: Field %s is missing in configured connection with alias %s!", keyPort, alias)
	}
	if _, ok := conn[keyToken]; !ok {
		return errors.Errorf("SpiceDB: Field %s is missing in configured connection with alias %s!", keyToken, alias)
	}
	insecure, ok := conn[keyInsecure]
	if !ok {
		return errors.Errorf("SpiceDB: Field %s is missing in configured connection with alias %s!", keyInsecure, alias)
	}
	if _, err := strconv.ParseBool(insecure); err != nil {
		return errors.Wrapf(err, "SpiceDB: Fiel %s is not a boolean in configured connection with alias %s!", keyInsecure, alias)
	}
	return nil
}

func getConnectionStringForPlatform(platform string, conn map[string]string) string {
	host, port, user, password, dbname, options := extractAndSortConnectionParameters(conn)

	switch platform {
	case data.TypePostgres:
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s%s", host, port, user, password, dbname, createConnOptionsString(options, " ", " "))
	case data.TypeMysql:
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", user, password, host, port, dbname, createConnOptionsString(options, "&", "?"))
	case data.TypeMongo:
		return fmt.Sprintf("mongodb://%s:%s@%s:%s/%s%s", user, password, host, port, dbname, createConnOptionsString(options, "&", "?"))
	default:
		logging.LogForComponent("datastore").Panic(fmt.Sprintf("Platform [%s] is not a supported DatastoreTranslator!", platform))
		return ""
	}
}

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

// Extract and sort all connection parameters by importance.
// Output: host, port, user, password, dbname, []options
// Each option has the format <key>=<value>
// nolint:gocritic
func extractAndSortConnectionParameters(conn map[string]string) (host, port, user, password, dbname string, options []string) {
	for key, value := range conn {
		switch key {
		case keyHost:
			host = value
		case keyPort:
			port = value
		case keyUser:
			user = value
		case keyPassword:
			password = value
		case keyDB:
			dbname = value
		default:
			options = append(options, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return host, port, user, password, dbname, options
}

func createConnOptionsString(options []string, delimiter, prefix string) string {
	optionString := strings.Join(options, delimiter)
	if len(options) > 0 {
		optionString = prefix + optionString
	}
	return optionString
}
