package request

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/request"
)

type urlProcessor struct {
	appConf    *configs.AppConfig
	config     *request.PathProcessorConfig
	configured bool
}

// URLProcessorInput needed to process a URL.
type URLProcessorInput struct {
	Method string
	URL    *url.URL
}

// NewURLProcessor returns a UrlProcessor instance implementing request.PathProcessor.
func NewURLProcessor() request.PathProcessor {
	return &urlProcessor{
		appConf:    nil,
		config:     nil,
		configured: false,
	}
}

func (processor *urlProcessor) Configure(appConf *configs.AppConfig, processorConf *request.PathProcessorConfig) error {
	// Exit if already configured
	if processor.configured {
		return nil
	}

	// Configure subcomponents
	if processorConf.PathMapper == nil {
		return errors.Errorf("UrlProcessor: PathMapper not configured! ")
	}
	mapper := *processorConf.PathMapper
	if err := mapper.Configure(appConf); err != nil {
		return err
	}

	processor.appConf = appConf
	processor.config = processorConf
	processor.configured = true
	logging.LogForComponent("urlProcessor").Infoln("Configured")
	return nil
}

func (processor *urlProcessor) Process(input any) (*request.PathProcessorOutput, error) {
	if !processor.configured {
		return nil, errors.Errorf("UrlProcessor was not configured! Please call Configure(). ")
	}
	if input == nil {
		return nil, errors.Errorf("UrlProcessor: Nil is no valid input for Process(). ")
	}

	// Check type and handle request
	switch in := input.(type) {
	case *URLProcessorInput:
		return processor.handleInput(in)
	default:
		return nil, errors.Errorf("urlProcessor: Input of Process() was not of type *request.URLProcessorInput! Type was: %s", reflect.TypeOf(input).String())
	}
}

func (processor *urlProcessor) handleInput(input *URLProcessorInput) (*request.PathProcessorOutput, error) {
	// Parse base pathSegments
	pathSegments := strings.Split(strings.Trim(input.URL.Path, "/"), "/")

	// Process query parameters
	queries := make(map[string]any)
	queryParams := input.URL.Query()
	for queryName := range queryParams {
		// Build queries which are passed to OPA as part of the input object
		queries[queryName] = queryParams.Get(queryName)
	}
	logging.LogForComponent("urlProcessor").Debugf("PathProcessor: Parsed path %+v with queries %+v", pathSegments, queries)

	// Map pathSegments and return
	out, err := (*processor.config.PathMapper).Map(&pathMapperInput{
		Method: input.Method,
		URL:    input.URL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "UrlProcessor: Error during path mapping.")
	}
	output := request.PathProcessorOutput{
		Datastores:     out.Datastores,
		Package:        out.Package,
		Authentication: out.Authentication,
		Authorization:  out.Authorization,
		Path:           pathSegments,
		Queries:        queries,
	}
	return &output, nil
}
