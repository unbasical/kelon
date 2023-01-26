package data

import (
	"embed"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/builtins"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"gopkg.in/yaml.v3"
)

//go:embed call-operands
var callOpsDir embed.FS

type GenericCallOpMapper struct {
	operator  string
	argsCount int
	handler   func(args ...string) (string, error)
}

func (h GenericCallOpMapper) Handles() string {
	return h.operator
}

func (h GenericCallOpMapper) Map(args ...string) (string, error) {
	argsLen := len(args)
	if argsLen < h.argsCount || argsLen > (h.argsCount+1) {
		return "", errors.Errorf("GenericCallOpMapper: Call-handler [%s] had wrong amount of arguments! Expected %d or %d arguments, but got %+v as input.", h.operator, h.argsCount, h.argsCount+1, args)
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
	return result, nil
}

type callHandlers struct {
	CallOperands []*loadedCallHandler `yaml:"call-operands"`
}

type loadedCallHandler struct {
	Operator      string `yaml:"op"`
	ArgsCount     int    `yaml:"args"`
	Mapping       string `yaml:"mapping"`
	Builtin       bool   `yaml:"builtin"`
	targetMapping string
	indexMapping  []int
}

func (h *loadedCallHandler) Handles() string {
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

func (h *loadedCallHandler) Map(args ...string) (string, error) {
	argsLen := len(args)
	if argsLen < h.ArgsCount || argsLen > (h.ArgsCount+1) {
		return "", errors.Errorf("GenericCallOpMapper: Call-Handler [%s] had wrong amount of arguments! Expected %d or %d arguments, but got %+v as input.", h.Operator, h.ArgsCount, h.ArgsCount+1, args)
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
		return fmt.Sprintf(h.targetMapping+" = %s", rearangedArgs...), nil
	}
	// Handle any other
	return fmt.Sprintf(h.targetMapping, rearangedArgs...), nil
}

// LoadAllCallOperands will try loading the call operands from the configured directory.
// If directory was not configured or and error while parsing occurred, the default call operands will be used.
func LoadAllCallOperands(dsConfs map[string]*configs.Datastore, callOperandsDir *string) (map[string]map[string]func(args ...string) (string, error), error) {
	operands := map[string]map[string]func(args ...string) (string, error){}
	dsFunctions := map[string]int{}

	for _, dsConf := range dsConfs {
		var defaultHandlers []data.CallOpMapper
		var customHandlers []data.CallOpMapper
		var parseErr error

		defaultHandlers, parseErr = loadDefaultDatastoreCallOps(dsConf.Type, dsFunctions)
		if parseErr != nil {
			return nil, errors.Wrapf(parseErr, "unable to load call operands for datastores of type %s", dsConf.Type)
		}

		if callOperandsDir != nil && *callOperandsDir != "" {
			callOpsFilePath := fmt.Sprintf("%s/%s.yml", *callOperandsDir, strings.ToLower(dsConf.Type))
			customHandlers, parseErr = loadDatastoreCallOpsFile(callOpsFilePath, dsFunctions)
			if parseErr != nil {
				logging.LogForComponent("callOperandsLoader").Warnf("failed loading custom call operands for %s. Only default call operands will be used: %s", dsConf.Type, parseErr.Error())
			}
		}

		ops := map[string]func(args ...string) (string, error){}
		for _, handler := range defaultHandlers {
			ops[handler.Handles()] = handler.Map
		}
		for _, handler := range customHandlers {
			ops[handler.Handles()] = handler.Map
		}

		operands[dsConf.Type] = ops
	}

	return operands, nil
}

func loadDatastoreCallOpsBytes(input []byte, dsFunctions map[string]int) ([]data.CallOpMapper, error) {
	if input == nil {
		return nil, errors.Errorf("Data must not be nil! ")
	}

	loadedConf := callHandlers{}
	// Load call operands
	if err := yaml.Unmarshal(input, &loadedConf); err != nil {
		return nil, errors.Errorf("GenericCallOpMapper: Unable to parse datastore call-operands config: " + err.Error())
	}

	result := make([]data.CallOpMapper, len(loadedConf.CallOperands))
	for i, h := range loadedConf.CallOperands {
		if h.Builtin {
			// Check function to be registered expects same args count as already registered function
			if argsCount, ok := dsFunctions[h.Operator]; ok && argsCount != h.ArgsCount {
				return nil, errors.Errorf("tried registering function %q with %d args but was already registered with %d args", h.Operator, argsCount, h.ArgsCount)
			}
			builtins.RegisterDatastoreFunction(h.Operator, h.ArgsCount)
			dsFunctions[h.Operator] = h.ArgsCount
		}

		if err := h.Init(); err != nil {
			return nil, errors.Wrap(err, "Error while loading call operands")
		}
		result[i] = h
	}

	return result, nil
}

func loadDatastoreCallOpsFile(filePath string, dsFunctions map[string]int) ([]data.CallOpMapper, error) {
	if filePath == "" {
		return nil, errors.Errorf("FilePath must not be empty! ")
	}

	// Load datastoreOpsBytes from file
	datastoreOpsBytes, ioError := os.ReadFile(filePath)
	if ioError == nil {
		return loadDatastoreCallOpsBytes(datastoreOpsBytes, dsFunctions)
	}
	return nil, errors.Wrap(ioError, "Unable to load datastore-call-operands")
}

func loadDefaultDatastoreCallOps(dsType string, dsFunctions map[string]int) ([]data.CallOpMapper, error) {
	filepath := fmt.Sprintf("call-operands/%s.yml", strings.ToLower(dsType))
	opsBytes, ioError := callOpsDir.ReadFile(filepath)
	if ioError != nil {
		return nil, errors.Wrapf(ioError, "unable to load default call-operands for datastore %q", dsType)
	}

	return loadDatastoreCallOpsBytes(opsBytes, dsFunctions)
}
