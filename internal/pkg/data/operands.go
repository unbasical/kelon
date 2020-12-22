package data

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"

	"github.com/Foundato/kelon/pkg/constants/logging"

	"github.com/Foundato/kelon/pkg/data"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type GenericCallOpMapper struct {
	operator  string
	argsCount int
	handler   func(args ...string) (string, error)
}

func (h GenericCallOpMapper) Handles() string {
	return h.operator
}

func (h GenericCallOpMapper) Map(args ...string) string {
	argsLen := len(args)
	if argsLen < h.argsCount || argsLen > (h.argsCount+1) {
		logging.LogForComponent("GenericCallOpMapper").Fatalf("Call-handler [%s] had wrong amount of arguments! Expected %d or %d arguments, but got %+v as input.", h.operator, h.argsCount, h.argsCount+1, args)
	}

	var (
		result string
		err    error
	)
	// Handle call with default comparison
	if argsLen > h.argsCount {
		result, err = h.handler(args[:argsLen-1]...)
		result = fmt.Sprintf("%s = %s", result, args[argsLen-1])
	} else { // Handle any other
		result, err = h.handler(args...)
	}

	if err != nil {
		logging.LogForComponent("GenericCallOpMapper").Fatalf("Call-handler [%s] failed due to error: %s", h.operator, err.Error())
	}
	return result
}

type callHandlers struct {
	CallOperands []*loadedCallHandler `yaml:"call-operands"`
}

type loadedCallHandler struct {
	Operator      string `yaml:"op"`
	ArgsCount     int    `yaml:"args"`
	Mapping       string
	targetMapping string
	indexMapping  []int
}

func (h loadedCallHandler) Handles() string {
	return h.Operator
}

func (h *loadedCallHandler) Init() error {
	argsMatcher := regexp.MustCompile(`\$\d+`)

	// Extract indices of operands
	h.indexMapping = []int{}
	extractedArgs := argsMatcher.FindAllString(h.Mapping, -1)
	for _, arg := range extractedArgs {
		num, err := strconv.Atoi(arg[1:])
		if err != nil {
			return errors.Wrap(err, "Unable to load datastore-call-operands")
		}
		h.indexMapping = append(h.indexMapping, num)
	}

	// Build target mapping
	h.targetMapping = argsMatcher.ReplaceAllString(h.Mapping, "%s")
	return nil
}

func (h loadedCallHandler) Map(args ...string) string {
	argsLen := len(args)
	if argsLen < h.ArgsCount || argsLen > (h.ArgsCount+1) {
		logging.LogForComponent("GenericCallOpMapper").Fatalf("Call-Handler [%s] had wrong amount of arguments! Expected %d or %d arguments, but got %+v as input.", h.Operator, h.ArgsCount, h.ArgsCount+1, args)
	}

	// Rearrange args if mapping is specified
	mapLen := len(h.indexMapping)
	rearangedArgs := make([]interface{}, argsLen)
	for i, arg := range args {
		if i < mapLen {
			rearangedArgs[i] = args[h.indexMapping[i]] // Rearrange
		} else {
			rearangedArgs[i] = arg // Append trailing
		}
	}

	// Handle call with default comparison
	if argsLen > h.ArgsCount {
		return fmt.Sprintf(h.targetMapping+" = %s", rearangedArgs...)
	}
	// Handle any other
	return fmt.Sprintf(h.targetMapping, rearangedArgs...)
}

func LoadDatastoreCallOpsBytes(input []byte) ([]data.CallOpMapper, error) {
	if input == nil {
		return nil, errors.New("Data must not be nil! ")
	}

	loadedConf := callHandlers{}
	// Load call operands
	if err := yaml.Unmarshal(input, &loadedConf); err != nil {
		return nil, errors.New("Unable to parse datastore call-operands config: " + err.Error())
	}

	result := make([]data.CallOpMapper, len(loadedConf.CallOperands))
	for i, h := range loadedConf.CallOperands {
		if err := h.Init(); err != nil {
			return nil, errors.Wrap(err, "Error while loading call operands")
		}
		result[i] = h
	}

	return result, nil
}

func LoadDatastoreCallOpsFile(filePath string) ([]data.CallOpMapper, error) {
	if filePath == "" {
		return nil, errors.New("FilePath must not be empty! ")
	}

	// Load datastoreOpsBytes from file
	datastoreOpsBytes, ioError := ioutil.ReadFile(filePath)
	if ioError == nil {
		return LoadDatastoreCallOpsBytes(datastoreOpsBytes)
	}
	return nil, errors.Wrap(ioError, "Unable to load datastore-call-operands")
}
