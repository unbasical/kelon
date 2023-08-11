// Package configs acts as the central place for app-global configuration.
package configs

import (
	"os"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/telemetry"
	"gopkg.in/yaml.v3"
)

// AppConfig represents the configuration for the entire app.
type AppConfig struct {
	ExternalConfig
	CallOperands    map[string]map[string]func(args ...string) (string, error)
	MetricsProvider telemetry.MetricsProvider
	TraceProvider   telemetry.TraceProvider
}

// ExternalConfig holds all externally configurable properties
type ExternalConfig struct {
	APIMappings       []*DatastoreAPIMapping              `yaml:"apis"`
	Datastores        map[string]*Datastore               `yaml:"datastores"`
	DatastoreSchemas  map[string]map[string]*EntitySchema `yaml:"entity_schemas"`
	JwtAuthenticators map[string]*JwtAuthentication       `yaml:"jwt"`
	OPA               interface{}                         `yaml:"opa"`
}

func (ec *ExternalConfig) Defaults() {
	for _, mapping := range ec.APIMappings {
		mapping.Defaults()
	}

	if ec.OPA == nil {
		ec.OPA = struct{}{}
	}
}

func (ec *ExternalConfig) Validate() error {
	for _, mapping := range ec.APIMappings {
		if err := mapping.Validate(ec.DatastoreSchemas); err != nil {
			return errors.Wrap(err, "loaded invalid configuration")
		}
	}

	return nil
}

// ConfigLoader is the interface that the functionality of loading kelon's external configuration.
type ConfigLoader interface {
	// Load loads all external configuration files from a predefined source.
	// It returns the loaded configuration and any error encountered that caused the Loader to stop early.
	Load() (*ExternalConfig, error)
}

// ByteConfigLoader implements Loader by loading config from provided bytes slices.
type ByteConfigLoader struct {
	FileBytes []byte
}

// Load implementation from ExternalLoader by using the properties of the ByteConfigLoader.
func (l ByteConfigLoader) Load() (*ExternalConfig, error) {
	if l.FileBytes == nil {
		return nil, errors.Errorf("config bytes must not be nil! ")
	}

	result := new(ExternalConfig)

	// Expand datastore config with environment variables
	l.FileBytes = []byte(os.ExpandEnv(string(l.FileBytes)))
	if err := yaml.Unmarshal(l.FileBytes, result); err != nil {
		return nil, errors.Errorf("Unable to parse struct of type %T: %s", result, err.Error())
	}

	result.Defaults()

	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// FileConfigLoader implements Loader by loading config from
// two files located at given paths.
type FileConfigLoader struct {
	FilePath string
}

// Load implementation from Loader by using the properties of the FileConfigLoader.
func (l FileConfigLoader) Load() (*ExternalConfig, error) {
	if l.FilePath == "" {
		return nil, errors.New("filepath must not be empty")
	}

	// Load configBy from file
	var (
		ioError   error
		fileBytes []byte
	)
	if fileBytes, ioError = os.ReadFile(l.FilePath); ioError != nil {
		return nil, ioError
	}

	return ByteConfigLoader{
		FileBytes: fileBytes,
	}.Load()
}
