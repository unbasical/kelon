package request

import (
	"errors"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"log"
)

type PathNotFoundError struct {
	MatchingDatastores []string
	Path               []string
}

func (e *PathNotFoundError) Error() string {
	return fmt.Sprintf("Path %v is ambiguous! Found %d possible datastores!", e.Path, len(e.MatchingDatastores))
}

type PathMapper interface {
	Configure(appConf *configs.AppConfig) error
	// Find datastore for mapped path and return (mappedPath, datastore, error)
	Map(path []string) ([]string, string, error)
}

type pathMapper struct {
	appConf    *configs.AppConfig
	configured bool
}

func NewPathMapper() PathMapper {
	return &pathMapper{
		appConf:    nil,
		configured: false,
	}
}

func (mapper *pathMapper) Configure(appConf *configs.AppConfig) error {
	if appConf == nil {
		return errors.New("PathMapper: AppConfig not configured! ")
	}
	mapper.appConf = appConf
	mapper.configured = true
	log.Println("Configured PathMapper")
	return nil
}

func (mapper pathMapper) Map(path []string) ([]string, string, error) {
	if !mapper.configured {
		return nil, "", errors.New("PathMapper was not configured! Please call Configure(). ")
	}
	if path == nil || len(path) == 0 {
		return nil, "", errors.New("PathMapper: Argument path mustn't be nil or empty! ")
	}

	// Search for custom mapping
	if match, matchDatastore, err := mapper.appConf.FindEntityMapping(path); err == nil && match != nil {
		return match, matchDatastore, nil
	} else if err == nil {
		// Otherwise just map entities
		possibleDatastores, _ := mapper.appConf.FindStoresContainingEntities(path)
		if len(possibleDatastores) == 1 {
			return mapEntities(path, possibleDatastores[0], mapper.appConf)
		} else {
			// More then one or zero datastores were found
			return nil, "", &PathNotFoundError{MatchingDatastores: possibleDatastores, Path: path}
		}
	} else {
		return nil, "", err
	}
}

func mapEntities(resources []string, datastore string, config *configs.AppConfig) ([]string, string, error) {
	var mappedEntities []string
	for _, res := range resources {
		if mapped, err := config.MapResource(datastore, res); err == nil {
			mappedEntities = append(mappedEntities, mapped)
		} else {
			return nil, "", err
		}
	}
	return mappedEntities, datastore, nil
}
