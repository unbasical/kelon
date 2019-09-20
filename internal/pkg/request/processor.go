package request

import (
	"github.com/Foundato/kelon/configs"
	"github.com/pkg/errors"
	"log"
	"net/url"
	"reflect"
	"strings"
)

type PathProcessorConfig struct {
	PathMapper *PathMapper
}

type PathProcessorOutput struct {
	Datastore string
	Entities  []string
	Path      []string
	Resources map[string]interface{}
}

type PathProcessor interface {
	Configure(appConf *configs.AppConfig, processorConf *PathProcessorConfig) error
	Process(input interface{}) (*PathProcessorOutput, error)
}

type urlProcessor struct {
	appConf    *configs.AppConfig
	config     *PathProcessorConfig
	configured bool
}

func NewUrlProcessor() PathProcessor {
	return &urlProcessor{
		appConf:    nil,
		config:     nil,
		configured: false,
	}
}

func (processor *urlProcessor) Configure(appConf *configs.AppConfig, processorConf *PathProcessorConfig) error {
	// Configure subcomponents
	if processorConf.PathMapper == nil {
		return errors.New("UrlProcessor: PathMapper not configured! ")
	}
	mapper := *processorConf.PathMapper
	if err := mapper.Configure(appConf); err != nil {
		return err
	}

	processor.appConf = appConf
	processor.config = processorConf
	processor.configured = true
	log.Println("Configured UrlProcessor")
	return nil
}

func (processor urlProcessor) Process(input interface{}) (*PathProcessorOutput, error) {
	if !processor.configured {
		return nil, errors.New("UrlProcessor was not configured! Please call Configure(). ")
	}
	if input == nil {
		return nil, errors.New("UrlProcessor: Nil is no valid input for Process(). ")
	}

	// Check type and handle request
	switch input.(type) {
	case *url.URL:
		return processor.handleInput(input.(*url.URL))
	default:
		return nil, errors.New("UrlProcessor: Input of Process() was not of type http.Request! Type was: " + reflect.TypeOf(input).String())
	}
}

func (processor urlProcessor) handleInput(inputURL *url.URL) (*PathProcessorOutput, error) {
	// Parse base path
	var path []string
	pathFields := strings.Fields(strings.ReplaceAll(strings.ToLower(inputURL.Path), "/", " "))
	path = append(path, pathFields...)

	// Process query parameters
	resources := make(map[string]interface{})
	queries := inputURL.Query()
	for queryName := range queries {
		// Append query parameter keys (also Resources)
		path = append(path, strings.ToLower(queryName))
		// Build resources which are passed to OPA as part of the input object
		resources[queryName] = queries.Get(queryName)
	}

	if processor.appConf.Debug {
		log.Printf("PathProcessor: Resource-Join is: %+v\n", path)
	}

	// Map path and return
	mapped, ds, err := (*processor.config.PathMapper).Map(path)
	if err != nil {
		return nil, errors.Wrap(err, "UrlProcessor: Error during path mapping.")
	}
	output := PathProcessorOutput{
		Datastore: ds,
		Entities:  mapped,
		Path:      pathFields,
		Resources: resources,
	}
	return &output, nil
}
