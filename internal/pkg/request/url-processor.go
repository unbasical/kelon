package request

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/Foundato/kelon/configs"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type urlProcessor struct {
	appConf    *configs.AppConfig
	config     *PathProcessorConfig
	configured bool
}

// Input needed to process a URL.
type UrlProcessorInput struct {
	Method string
	Url    *url.URL
}

// Return a UrlProcessor instance implementing request.PathProcessor.
func NewUrlProcessor() PathProcessor {
	return &urlProcessor{
		appConf:    nil,
		config:     nil,
		configured: false,
	}
}

// See request.PathProcessor.
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
	log.Infoln("Configured UrlProcessor")
	return nil
}

// See request.PathProcessor.
func (processor urlProcessor) Process(input interface{}) (*PathProcessorOutput, error) {
	if !processor.configured {
		return nil, errors.New("UrlProcessor was not configured! Please call Configure(). ")
	}
	if input == nil {
		return nil, errors.New("UrlProcessor: Nil is no valid input for Process(). ")
	}

	// Check type and handle request
	switch in := input.(type) {
	case *UrlProcessorInput:
		return processor.handleInput(in)
	default:
		return nil, errors.New("UrlProcessor: Input of Process() was not of type *request.UrlProcessorInput! Type was: " + reflect.TypeOf(input).String())
	}
}

func (processor urlProcessor) handleInput(input *UrlProcessorInput) (*PathProcessorOutput, error) {
	// Parse base path
	path := strings.Fields(strings.ReplaceAll(strings.ToLower(input.Url.Path), "/", " "))
	// Process query parameters
	queries := make(map[string]interface{})
	queryParams := input.Url.Query()
	for queryName := range queryParams {
		// Build queries which are passed to OPA as part of the input object
		queries[queryName] = queryParams.Get(queryName)
	}
	log.Debugf("PathProcessor: Parsed path %+v with queries %+v\n", path, queries)

	// Map path and return
	out, err := (*processor.config.PathMapper).Map(&pathMapperInput{
		Method: input.Method,
		Url:    input.Url,
	})
	if err != nil {
		return nil, errors.Wrap(err, "UrlProcessor: Error during path mapping.")
	}
	output := PathProcessorOutput{
		Datastore: out.Datastore,
		Package:   out.Package,
		Path:      path,
		Queries:   queries,
	}
	return &output, nil
}
