package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"strings"
)

type policyCompiler struct {
	configured bool
	appConfig  *configs.AppConfig
	config     *PolicyCompilerConfig
	engine     *OPA
}

func NewPolicyCompiler() PolicyCompiler {
	return &policyCompiler{
		configured: false,
		config:     nil,
	}
}

func (compiler *policyCompiler) Configure(appConf *configs.AppConfig, compConf *PolicyCompilerConfig) error {
	if e := initDependencies(compConf, appConf); e != nil {
		return errors.Wrap(e, "PolicyCompiler: Error while initializing dependencies.")
	}

	// Start OPA in background
	engine, err := startOPA(compConf.OpaConfigPath)
	if err != nil {
		return errors.Wrap(err, "PolicyCompiler: Error while starting OPA.")
	}

	// Assign variables
	compiler.engine = engine
	compiler.appConfig = appConf
	compiler.config = compConf
	compiler.configured = true
	log.Infoln("Configured PolicyCompiler")
	return nil
}

func (compiler policyCompiler) Process(request *http.Request) (bool, error) {
	if !compiler.configured {
		return false, errors.New("PolicyCompiler was not configured! Please call Configure(). ")
	}

	// Parse body of request
	requestBody := make(map[string]interface{})
	if marshalErr := json.NewDecoder(request.Body).Decode(&requestBody); marshalErr != nil {
		return false, errors.Wrap(marshalErr, "PolicyCompiler: Error while parsing request body!")
	}

	// Process path
	output, err := compiler.processPath(requestBody)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during path processing")
	}

	// Compile mapped path
	queries, err := compiler.opaCompile(request, &requestBody, output)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during policy compilation")
	}
	if done := anyQuerySucceeded(queries); done {
		return true, nil
	}

	// Otherwise translate ast
	result, err := (*compiler.config.Translator).Process(queries, output.Datastore)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during ast translation")
	}

	// If we receive something from the datastore, the query was successful
	return result, nil
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

func (compiler policyCompiler) processPath(requestBody map[string]interface{}) (*request.PathProcessorOutput, error) {
	inputURL, err := extractUrlFromRequestBody(requestBody)
	if err != nil {
		return nil, err
	}
	var method string
	if sentMethod, ok := requestBody["method"]; ok {
		if m, ok := sentMethod.(string); ok {
			method = strings.ToUpper(m)
		} else {
			return nil, errors.Errorf("PolicyCompiler: Attribute 'method' of request body was not of type string! Type was %T\n", sentMethod)
		}
	} else {
		return nil, errors.New("PolicyCompiler: Request body didn't contain a 'method'. ")
	}

	output, err := (*compiler.config.PathProcessor).Process(&request.UrlProcessorInput{
		Method: method,
		Url:    inputURL,
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("Mapped request [%s] to: Datastores [%s] Package: [%s]\n", inputURL, output.Datastore, output.Package)
	return output, nil
}

func (compiler *policyCompiler) opaCompile(clientRequest *http.Request, requestBody *map[string]interface{}, output *request.PathProcessorOutput) (*rego.PartialQueries, error) {
	// Extract parameters for partial evaluation
	opts := compiler.extractOpaOpts(output)
	input := extractOpaInput(output, requestBody)
	query := fmt.Sprintf("data.%s.allow == true", output.Package)

	// Compile clientRequest and return answer
	queries, err := compiler.engine.PartialEvaluate(clientRequest.Context(), input, query, opts...)
	if err == nil {
		if log.IsLevelEnabled(log.DebugLevel) {
			log.Debugf("=======> OPA's Partial Evaluation with input: \n%+v\nReturned queries:\n", input)
			for _, q := range queries.Queries {
				log.Debugf("[%+v]\n", q)
			}
		}
		return queries, nil
	} else {
		return nil, err
	}
}

func extractUrlFromRequestBody(requestBody map[string]interface{}) (*url.URL, error) {
	if sentPath, ok := requestBody["path"]; ok {
		if sentURL, ok := sentPath.(string); ok {
			if parsed, urlError := url.Parse(sentURL); urlError == nil {
				return parsed, nil
			} else {
				return nil, errors.Wrap(urlError, "PolicyCompiler: Field 'path' from request body is no valid URL")
			}
		} else {
			return nil, errors.Errorf("PolicyCompiler: Attribute 'path' of request body was not of type string! Type was %T\n", sentURL)
		}
	} else {
		return nil, errors.New("PolicyCompiler: Request body didn't contain a 'path'. ")
	}
}

func (compiler *policyCompiler) extractOpaOpts(output *request.PathProcessorOutput) []func(*rego.Rego) {
	unknowns := []string{fmt.Sprintf("data.%s", output.Datastore)}
	opts := []func(*rego.Rego){
		rego.Unknowns(unknowns),
	}
	log.Debugf("Sending unknowns %+v\n", unknowns)

	if regos := compiler.config.RegoPaths; regos != nil {
		log.Debugf("Loaded rego: %+v\n", *regos)
		opts = append(opts, rego.Load(*regos, nil))
	}
	return opts
}

func extractOpaInput(output *request.PathProcessorOutput, requestBody *map[string]interface{}) map[string]interface{} {
	input := map[string]interface{}{
		"queries": output.Queries,
	}
	// Append custom fields to received body
	for key, value := range *requestBody {
		input[key] = value
	}

	// Add parsed input without query params
	input["path"] = output.Path
	return input
}

func initDependencies(compConf *PolicyCompilerConfig, appConf *configs.AppConfig) error {
	// Configure PathProcessor
	if compConf.PathProcessor == nil {
		return errors.New("PolicyCompiler: PathProcessor not configured! ")
	}
	parser := *compConf.PathProcessor
	if err := parser.Configure(appConf, &compConf.PathProcessorConfig); err != nil {
		return err
	}
	// Configure AstTranslator
	if compConf.Translator == nil {
		return errors.New("PolicyCompiler: Translator not configured! ")
	}
	translator := *compConf.Translator
	if err := translator.Configure(appConf, &compConf.AstTranslatorConfig); err != nil {
		return err
	}
	return nil
}

func startOPA(configFile *string) (*OPA, error) {
	engine, err := NewOPA(ConfigOPA(*configFile))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize OPA!")
	}

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "Failed to start OPA!")
	}

	return engine, nil
}
