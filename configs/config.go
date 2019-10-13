// Central package for app-global configs.
package configs

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Configuration for the entire app.
type AppConfig struct {
	ExternalConfig
}

// External configs.
type ExternalConfig struct {
	Data *DatastoreConfig
	Api  *ApiConfig
}

// ConfigLoader is the interface that the functionality of loading kelon's external configuration.
//
// Load loads all external configuration files from a predefined source.
// It returns the loaded configuration and any error encountered that caused the Loader to stop early.
type ConfigLoader interface {
	Load() (*ExternalConfig, error)
}

// ByteConfigLoader implements configs.ConfigLoader by loading configs from
// two provided bytes slices.
type ByteConfigLoader struct {
	DatastoreConfigBytes []byte
	ApiConfigBytes       []byte
}

// Implementing Load from configs.ConfigLoader by using the properties of the ByteConfigLoader.
func (l ByteConfigLoader) Load() (*ExternalConfig, error) {
	if l.DatastoreConfigBytes == nil {
		return nil, errors.New("DatastoreConfigBytes must not be nil! ")
	}
	if l.ApiConfigBytes == nil {
		return nil, errors.New("ApiConfigBytes must not be nil! ")
	}

	result := new(ExternalConfig)

	// Load datastore config
	result.Data = new(DatastoreConfig)
	if err := yaml.Unmarshal(l.DatastoreConfigBytes, result.Data); err != nil {
		return nil, errors.New("Unable to parse datastore config: " + err.Error())
	}

	// Load API config
	result.Api = new(ApiConfig)
	if err := yaml.Unmarshal(l.ApiConfigBytes, result.Api); err != nil {
		return nil, errors.New("Unable to parse api config: " + err.Error())
	}

	return result, nil
}

// FileConfigLoader implements configs.ConfigLoader by loading configs from
// two files located at given paths.
type FileConfigLoader struct {
	DatastoreConfigPath string
	ApiConfigPath       string
}

// Implementing Load from configs.ConfigLoader by using the properties of the FileConfigLoader.
func (l FileConfigLoader) Load() (*ExternalConfig, error) {
	if l.DatastoreConfigPath == "" {
		return nil, errors.New("DatastoreConfigPath must not be empty! ")
	}
	if l.ApiConfigPath == "" {
		return nil, errors.New("ApiConfigPath must not be empty! ")
	}

	// Load dsConfigBytes from file
	if dsConfigBytes, ioError := ioutil.ReadFile(l.DatastoreConfigPath); ioError == nil {
		if apiConfigBytes, ioError := ioutil.ReadFile(l.ApiConfigPath); ioError == nil {
			return ByteConfigLoader{
				DatastoreConfigBytes: dsConfigBytes,
				ApiConfigBytes:       apiConfigBytes,
			}.Load()
		} else {
			return nil, ioError
		}
	} else {
		return nil, ioError
	}
}
