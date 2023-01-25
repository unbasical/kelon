package opa

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	requestInt "github.com/unbasical/kelon/internal/pkg/request"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	internalErrors "github.com/unbasical/kelon/pkg/errors"
	"github.com/unbasical/kelon/pkg/opa"
	"github.com/unbasical/kelon/pkg/request"
	"github.com/unbasical/kelon/pkg/watcher"
)

type policyCompiler struct {
	configured bool
	appConfig  *configs.AppConfig
	config     *opa.PolicyCompilerConfig
	engine     *OPA
}

// Return a new instance of the default implementation of the opa.PolicyCompiler.
func NewPolicyCompiler() opa.PolicyCompiler {
	return &policyCompiler{
		configured: false,
		config:     nil,
	}
}

// See GetEngine() from opa.PolicyCompiler
func (compiler *policyCompiler) GetEngine() *plugins.Manager {
	return compiler.engine.manager
}

// See Configure() from opa.PolicyCompiler
func (compiler *policyCompiler) Configure(appConf *configs.AppConfig, compConf *opa.PolicyCompilerConfig) error {
	// Exit if already configured
	if compiler.configured {
		return nil
	}

	if e := initDependencies(compConf, appConf); e != nil {
		return errors.Wrap(e, "PolicyCompiler: Error while initializing dependencies.")
	}

	// Start OPA in background
	engine, err := startOPA(compConf.OPAConfig, *compConf.RegoDir)
	if err != nil {
		return errors.Wrap(err, "PolicyCompiler: Error while starting OPA.")
	}

	// Register watcher for rego changes
	(*compConf.ConfigWatcher).Watch(func(changeType watcher.ChangeType, config *configs.ExternalConfig, e error) {
		if changeType == watcher.ChangeRego {
			if err := engine.LoadRegosFromPath(context.Background(), *compConf.RegoDir); err != nil {
				logging.LogForComponent("policyCompiler").Error("Unable to reload regos on file change due to: ", err)
			}
		}
	})

	// Assign variables
	compiler.engine = engine
	compiler.appConfig = appConf
	compiler.config = compConf
	compiler.configured = true
	logging.LogForComponent("policyCompiler").Infoln("Configured PolicyCompiler")
	return nil
}

func (compiler policyCompiler) Execute(ctx context.Context, requestBody map[string]interface{}) (*opa.Decision, error) {
	// Validate if policy compiler was configured correctly
	if !compiler.configured {
		return nil, errors.Errorf("PolicyCompiler was not configured! Please call Configure(). ")
	}

	// Extract input
	for rootKey := range requestBody {
		if rootKey != "input" {
			logging.LogForComponent("policyCompiler").Warnf("Request field %q which will be ignored!", rootKey)
		}
	}

	rawInput, exists := requestBody["input"]
	if !exists {
		return nil, internalErrors.InvalidInput{Msg: "PolicyCompiler: Incoming requestBody had no field 'input'!"}
	}

	input, ok := rawInput.(map[string]interface{})
	if !ok {
		return nil, internalErrors.InvalidInput{Msg: "PolicyCompiler: Field 'input' in requestBody body was no nested JSON object!"}
	}
	logging.LogForComponent("policyCompiler").Debugf("Received input: %+v", input)

	// Process path
	output, err := compiler.processPath(input)
	if err != nil {
		return nil, err
	}

	path, err := extractURLFromRequestBody(input)
	if err != nil {
		return nil, err
	}

	method, err := extractMethodFromRequestBody(input)
	if err != nil {
		return nil, err
	}

	var verify = true
	var allow = true

	// Authentication
	if output.Authentication {
		verify, err = compiler.evalFunction(ctx, "verify", input, output)
		if err != nil {
			return &opa.Decision{Verify: false, Allow: false, Package: output.Package, Method: method, Path: path.String()}, err
		}
	}

	// Authorization
	if verify {
		if output.Authorization {
			allow, err = compiler.evalFunction(ctx, "allow", input, output)
			if err != nil {
				return &opa.Decision{Verify: true, Allow: false, Package: output.Package, Method: method, Path: path.String()}, err
			}
		}
	} else {
		allow = false
	}

	return &opa.Decision{Verify: verify, Allow: allow, Package: output.Package, Method: method, Path: path.String()}, nil
}

func anyQuerySucceeded(queries *rego.PartialQueries) bool {
	// If there are no queries, we are done
	if len(queries.Queries) == 0 {
		return true
	}
	// Or if there is one query which succeeded
	for _, q := range queries.Queries {
		if len(q) == 0 {
			return true
		}
	}

	return false
}

func (compiler policyCompiler) processPath(input map[string]interface{}) (*request.PathProcessorOutput, error) {
	inputURL, err := extractURLFromRequestBody(input)
	if err != nil {
		return nil, err
	}
	method, err := extractMethodFromRequestBody(input)
	if err != nil {
		return nil, err
	}

	output, err := (*compiler.config.PathProcessor).Process(&requestInt.URLProcessorInput{
		Method: method,
		URL:    inputURL,
	})
	if err != nil {
		return nil, err
	}
	logging.LogForComponent("policyCompiler").Debugf("Mapped request [%s] to: Datastores [%+v] Package: [%s]", inputURL, output.Datastores, output.Package)
	return output, nil
}

func (compiler *policyCompiler) evalFunction(ctx context.Context, function string, input map[string]interface{}, output *request.PathProcessorOutput) (bool, error) {
	// Compile mapped path
	queries, err := compiler.opaCompile(ctx, input, function, output)
	if err != nil {
		return false, err
	}

	// OPA decided denied
	if queries.Queries == nil {
		return false, nil
	}
	// Check if any query succeeded
	if done := anyQuerySucceeded(queries); done {
		return true, nil
	}

	// Otherwise translate ast
	return (*compiler.config.Translator).Process(context.WithValue(ctx, constants.ContextKeyRegoPackage, output.Package), queries, output.Datastores)
}

func (compiler *policyCompiler) opaCompile(ctx context.Context, input map[string]interface{}, function string, output *request.PathProcessorOutput) (*rego.PartialQueries, error) {
	// Extract parameters for partial evaluation
	opts := compiler.extractOpaOpts(output)
	extractedInput := extractOpaInput(output, input)
	query := fmt.Sprintf("data.%s.%s == true", output.Package, function)
	logging.LogForComponent("policyCompiler").Debugf("Sending query=%s", query)

	// Compile clientRequest and return answer
	queries, err := compiler.engine.PartialEvaluate(ctx, extractedInput, query, opts...)
	if err == nil {
		if log.IsLevelEnabled(log.DebugLevel) {
			for _, q := range queries.Queries {
				log.Debugf("[%+v]", q)
			}
		}
		return queries, nil
	}
	return nil, err
}

func extractMethodFromRequestBody(input map[string]interface{}) (string, error) {
	if inputMethod, ok := input["method"]; ok {
		if m, ok := inputMethod.(string); ok {
			return strings.ToUpper(m), nil
		}
		return "", internalErrors.InvalidInput{Msg: fmt.Sprintf("PolicyCompiler: Attribute 'method' of request body was not of type string! Type was %T", inputMethod)}
	}
	return "", internalErrors.InvalidInput{Msg: "PolicyCompiler: Object 'input' of request body didn't contain a 'method'"}
}

func extractURLFromRequestBody(input map[string]interface{}) (*url.URL, error) {
	sentPath, hasPath := input["path"]
	if hasPath {
		switch sentURL := sentPath.(type) {
		case string:
			parsed, urlError := url.Parse(sentURL)
			if urlError == nil {
				return parsed, nil
			}
			return nil, internalErrors.InvalidInput{Cause: urlError, Msg: "PolicyCompiler: Field 'path' from request body is no valid URL"}
		case []interface{}:
			stringedURL := make([]string, len(sentURL))
			for i, v := range sentURL {
				stringedURL[i] = fmt.Sprint(v)
			}
			parsed, urlError := url.Parse("/" + strings.Join(stringedURL, "/"))
			if urlError == nil {
				return parsed, nil
			}
			return nil, internalErrors.InvalidInput{Cause: urlError, Msg: "PolicyCompiler: Field 'path' from request body is no valid URL"}
		default:
			return nil, internalErrors.InvalidInput{Msg: fmt.Sprintf("PolicyCompiler: Attribute 'path' of request body was not of type string! Type was %T", sentURL)}
		}
	}
	return nil, internalErrors.InvalidInput{Msg: "PolicyCompiler: Object 'input' of request body didn't contain a 'path'. "}
}

func (compiler *policyCompiler) extractOpaOpts(output *request.PathProcessorOutput) []func(*rego.Rego) {
	unknowns := make([]string, len(output.Datastores))
	for i, datastore := range output.Datastores {
		unknowns[i] = fmt.Sprintf("data.%s", datastore)
	}
	logging.LogForComponent("policyCompiler").Debugf("Sending unknowns %+v", unknowns)
	return []func(*rego.Rego){
		rego.Unknowns(unknowns),
	}
}

func extractOpaInput(output *request.PathProcessorOutput, input map[string]interface{}) map[string]interface{} {
	extracted := map[string]interface{}{
		"queries": output.Queries,
	}
	// Append custom fields to received body
	for key, value := range input {
		extracted[key] = value
	}

	// Add parsed extracted without query params
	extracted["path"] = output.Path
	return extracted
}

func initDependencies(compConf *opa.PolicyCompilerConfig, appConf *configs.AppConfig) error {
	// Configure PathProcessor
	if compConf.PathProcessor == nil {
		return errors.Errorf("PolicyCompiler: PathProcessor not configured!")
	}
	parser := *compConf.PathProcessor
	if err := parser.Configure(appConf, &compConf.PathProcessorConfig); err != nil {
		return err
	}
	// Configure AstTranslator
	if compConf.Translator == nil {
		return errors.Errorf("PolicyCompiler: Translator not configured!")
	}
	translator := *compConf.Translator
	if err := translator.Configure(appConf, &compConf.AstTranslatorConfig); err != nil {
		return err
	}
	return nil
}

func startOPA(conf interface{}, regosPath string) (*OPA, error) {
	ctx := context.Background()
	engine, err := NewOPA(ctx, regosPath, ConfigOPA(conf))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize OPA!")
	}

	if err := engine.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "Failed to start OPA!")
	}

	return engine, nil
}
