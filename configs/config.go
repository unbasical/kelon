package configs

import (
	"errors"
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

type Config struct {
	Data *DatastoreConfig
	Api  map[string]*ApiConfig
}

type ConfigLoader interface {
	Load() (*Config, error)
}

type ByteConfigLoader struct {
	DatastoreConfigBytes []byte
	ApiConfigBytes       []byte
}

func (l ByteConfigLoader) Load() (*Config, error) {
	if l.DatastoreConfigBytes == nil {
		return nil, errors.New("DatastoreConfigBytes must not be nil!")
	}
	if l.ApiConfigBytes == nil {
		return nil, errors.New("ApiConfigBytes must not be nil!")
	}

	result := new(Config)

	// Load datastore config
	result.Data = new(DatastoreConfig)
	if err := yaml.Unmarshal(l.DatastoreConfigBytes, result.Data); err != nil {
		return nil, errors.New("Unable to parse datastore config: " + err.Error())
	}

	// Load API config
	result.Api = make(map[string]*ApiConfig)
	if err := yaml.Unmarshal(l.ApiConfigBytes, result.Api); err != nil {
		return nil, errors.New("Unable to parse api config: " + err.Error())
	}

	return result, nil
}

type FileConfigLoader struct {
	DatastoreConfigPath string
	ApiConfigPath       string
}

func (l FileConfigLoader) Load() (*Config, error) {
	if l.DatastoreConfigPath == "" {
		return nil, errors.New("DatastoreConfigPath must not be empty!")
	}
	if l.ApiConfigPath == "" {
		return nil, errors.New("ApiConfigPath must not be empty!")
	}

	// Load data from file
	if data, ioError := ioutil.ReadFile(l.DatastoreConfigPath); ioError == nil {
		if api, ioError := ioutil.ReadFile(l.ApiConfigPath); ioError == nil {
			return ByteConfigLoader{
				DatastoreConfigBytes: data,
				ApiConfigBytes:       api,
			}.Load()
		} else {
			return nil, ioError
		}
	} else {
		return nil, ioError
	}
}
