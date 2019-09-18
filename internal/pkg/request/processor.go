package request

import (
	"errors"
	"github.com/Foundato/kelon/configs"
	"log"
	"net/http"
	"reflect"
)

type PathProcessorConfig struct {
	PathMapper *PathMapper
}

type PathProcessor interface {
	Configure(appConf *configs.AppConfig, processorConf *PathProcessorConfig) error
	Process(input interface{}) ([]string, error)
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

func (processor urlProcessor) Process(input interface{}) ([]string, error) {
	if !processor.configured {
		return nil, errors.New("UrlProcessor was not configured! Please call Configure(). ")
	}
	if input == nil {
		return nil, errors.New("UrlProcessor: Nil is no valid input for Process(). ")
	}

	// Check type and handle request
	switch input.(type) {
	case *http.Request:
		return processor.handleInput(input.(*http.Request))
	default:
		return nil, errors.New("UrlProcessor: Input of Process() was not of type http.Request! Type was: " + reflect.TypeOf(input).String())
	}
}

func (processor urlProcessor) handleInput(request *http.Request) ([]string, error) {
	// TODO implement Request-parsing
	return []string{}, nil
}
