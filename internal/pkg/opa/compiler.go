package opa

import (
	"context"
	"encoding/json"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/internal/pkg/translate"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type CompilerConfig struct {
	OpaConfigPath *string
	RegoPaths     *[]string
	Prefix        *string
	PathProcessor *request.PathProcessor
	Translator    *translate.AstTranslator
	translate.AstTranslatorConfig
	request.PathProcessorConfig
}

type PolicyCompiler interface {
	Configure(appConfig *configs.AppConfig, compConfig *CompilerConfig) error
	Process(request *http.Request) (bool, error)
}

type policyCompiler struct {
	configured bool
	appConfig  *configs.AppConfig
	config     *CompilerConfig
	engine     *OPA
}

func NewPolicyCompiler() PolicyCompiler {
	return &policyCompiler{
		configured: false,
		config:     nil,
	}
}

func (compiler *policyCompiler) Configure(appConf *configs.AppConfig, compConf *CompilerConfig) error {
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
	log.Println("Configured PolicyCompiler")
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
	result, err := (*compiler.config.Translator).Process(queries)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during ast translation")
	}

	// If we receive something from the datastore, the query was successful
	return result != nil && len(*result) > 0, nil
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
	output, err := (*compiler.config.PathProcessor).Process(inputURL)
	if err != nil {
		return nil, err
	}
	if compiler.appConfig.Debug {
		log.Printf("Datastore [%s] -> entities: %v\n", output.Datastore, output.Entities)
	}
	return output, nil
}

func (compiler *policyCompiler) opaCompile(clientRequest *http.Request, requestBody *map[string]interface{}, output *request.PathProcessorOutput) (*rego.PartialQueries, error) {
	// Extract parameters for partial evaluation
	query := compiler.extractOpaQuery(clientRequest)
	opts := compiler.extractOpaOpts(output)
	input := extractOpaInput(output, requestBody)

	// Compile clientRequest and return answer
	queries, err := compiler.engine.PartialEvaluate(clientRequest.Context(), input, query, opts...)
	if err == nil {
		if compiler.appConfig.Debug {
			log.Printf("OPA's Partial Evaluation returned: %+v\n", queries.Queries)
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
	var unknowns []string
	for _, entity := range output.Entities {
		unknowns = append(unknowns, "data."+entity)
	}
	if compiler.appConfig.Debug {
		log.Printf("Unknowns sent to OPA are: %+v\n", unknowns)
	}

	opts := []func(*rego.Rego){
		rego.Unknowns(unknowns),
	}
	if regos := compiler.config.RegoPaths; regos != nil {
		opts = append(opts, rego.Load(*regos, nil))
	}
	return opts
}

func extractOpaInput(output *request.PathProcessorOutput, requestBody *map[string]interface{}) map[string]interface{} {
	input := map[string]interface{}{
		"resources": output.Resources,
	}
	// Append custom fields to received body
	for key, value := range *requestBody {
		input[key] = value
	}

	// Add parsed input without query params
	input["path"] = output.Path
	return input
}

func (compiler *policyCompiler) extractOpaQuery(r *http.Request) string {
	query := r.URL.Path
	query = strings.ReplaceAll(query, *compiler.config.Prefix, "")
	query = strings.ReplaceAll(query, "/", " ")
	query = strings.Join(strings.Fields(query), ".")
	query += ".allow == true"
	return query
}

func initDependencies(compConf *CompilerConfig, appConf *configs.AppConfig) error {
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
