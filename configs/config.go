// Central package for app-global config.
package configs

import (
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/telemetry"
	"gopkg.in/yaml.v3"
)

// Configuration for the entire app.
type AppConfig struct {
	ExternalConfig
	TelemetryProvider telemetry.Provider
}

// External config.
type ExternalConfig struct {
	Data *DatastoreConfig
	API  *APIConfig
}

// ConfigLoader is the interface that the functionality of loading kelon's external configuration.
//
// Load loads all external configuration files from a predefined source.
// It returns the loaded configuration and any error encountered that caused the Loader to stop early.
type ConfigLoader interface {
	Load() (*ExternalConfig, error)
}

// ByteConfigLoader implements configs.ConfigLoader by loading config from
// two provided bytes slices.
type ByteConfigLoader struct {
	DatastoreConfigBytes []byte
	APIConfigBytes       []byte
}

// Implementing Load from configs.ConfigLoader by using the properties of the ByteConfigLoader.
func (l ByteConfigLoader) Load() (*ExternalConfig, error) {
	if l.DatastoreConfigBytes == nil {
		return nil, errors.Errorf("DatastoreConfigBytes must not be nil!")
	}
	if l.APIConfigBytes == nil {
		return nil, errors.Errorf("APIConfigBytes must not be nil! ")
	}

	result := new(ExternalConfig)

	// Load datastore config
	result.Data = new(DatastoreConfig)
	// Expand datastore config with environment variables
	l.DatastoreConfigBytes = []byte(os.ExpandEnv(string(l.DatastoreConfigBytes)))
	if err := yaml.Unmarshal(l.DatastoreConfigBytes, result.Data); err != nil {
		return nil, errors.Errorf("Unable to parse datastore config: " + err.Error())
	}

	// Load API config
	result.API = new(APIConfig)
	if err := yaml.Unmarshal(l.APIConfigBytes, result.API); err != nil {
		return nil, errors.Errorf("Unable to parse api config: " + err.Error())
	}

	// Validate config
	if err := result.Data.validate(); err != nil {
		return nil, errors.Wrap(err, "Loaded invalid datastore config")
	}

	return result, nil
}

// FileConfigLoader implements configs.ConfigLoader by loading config from
// two files located at given paths.
type FileConfigLoader struct {
	DatastoreConfigPath string
	APIConfigPath       string
}

// Implementing Load from configs.ConfigLoader by using the properties of the FileConfigLoader.
func (l FileConfigLoader) Load() (*ExternalConfig, error) {
	if l.DatastoreConfigPath == "" {
		return nil, errors.Errorf("DatastoreConfigPath must not be empty!")
	}
	if l.APIConfigPath == "" {
		return nil, errors.Errorf("APIConfigPath must not be empty! ")
	}

	// Load dsConfigBytes from file
	var (
		ioError        error
		dsConfigBytes  []byte
		apiConfigBytes []byte
	)
	if dsConfigBytes, ioError = ioutil.ReadFile(l.DatastoreConfigPath); ioError == nil {
		if apiConfigBytes, ioError = ioutil.ReadFile(l.APIConfigPath); ioError == nil {
			return ByteConfigLoader{
				DatastoreConfigBytes: dsConfigBytes,
				APIConfigBytes:       apiConfigBytes,
			}.Load()
		}
	}
	return nil, ioError
}
