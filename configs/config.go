package configs

import (
	"errors"
	"github.com/Foundato/kelon/internal/pkg/util"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
)

type AppConfig struct {
	ExternalConfig
	Debug bool
}

type ExternalConfig struct {
	Data *DatastoreConfig
	Api  map[string]*ApiConfig
}

type ConfigLoader interface {
	Load() (*ExternalConfig, error)
}

type ByteConfigLoader struct {
	DatastoreConfigBytes []byte
	ApiConfigBytes       []byte
}

func (l ByteConfigLoader) Load() (*ExternalConfig, error) {
	if l.DatastoreConfigBytes == nil {
		return nil, errors.New("DatastoreConfigBytes must not be nil!")
	}
	if l.ApiConfigBytes == nil {
		return nil, errors.New("ApiConfigBytes must not be nil!")
	}

	result := new(ExternalConfig)

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

func (l FileConfigLoader) Load() (*ExternalConfig, error) {
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

// Find datastore for mapped path and return (mappedPath, datastore, error)
func (conf *AppConfig) FindEntityMapping(path []string) ([]string, string, error) {
	if path == nil || len(path) == 0 {
		return nil, "", errors.New("AppConfig: Argument path of method FindEntityMapping() mustn't be nil or empty! ")
	}

	searchJoin := strings.ToLower(strings.Join(path, "."))
	for datastore, val := range conf.Api {
		for resourceJoin, entityJoin := range val.JoinPaths {
			if searchJoin == strings.ToLower(resourceJoin) {
				return entityJoin, datastore, nil
			}
		}
	}

	return nil, "", nil
}

func (conf *AppConfig) FindStoresContainingEntities(entities []string) ([]string, error) {
	if entities == nil || len(entities) == 0 {
		return nil, errors.New("AppConfig: Argument entities of method FindStoresContainingEntities() mustn't be nil or empty! ")
	}

	var matchingDatastores []string
	for datastore, apiConfig := range conf.Api {
		if util.AllContained(apiConfig.ResourcePools, entities) {
			matchingDatastores = append(matchingDatastores, datastore)
		}
	}

	return matchingDatastores, nil
}

func (conf *AppConfig) MapResource(datasource string, resource string) (string, error) {
	if datasource == "" || resource == "" {
		return "", errors.New("AppConfig: Empty string is not an valid argument for MapResource()! ")
	}
	if _, ok := conf.Api[datasource]; !ok {
		return "", errors.New("AppConfig: Datastore provided in MapResource() is not contained in api-config! ")
	}

	// First: map grouped resources
	for groupName, group := range conf.Api[datasource].ResourcePoolGroups {
		for _, resourceName := range group.Resources {
			if resourceName == resource {
				resource = groupName
			}
		}
	}

	// Then map resource via link if available
	for resourceName, entityName := range conf.Api[datasource].ResourceLinks {
		if resourceName == resource {
			return entityName, nil
		}
	}

	// Or return resource again if no links are available
	return resource, nil
}
