package opa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/open-policy-agent/opa/plugins"

	"github.com/Foundato/kelon/pkg/watcher"

	"github.com/Foundato/kelon/configs"
	requestInt "github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/pkg/opa"
	"github.com/Foundato/kelon/pkg/request"
	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
	engine, err := startOPA(compConf.OpaConfigPath, *compConf.RegoDir)
	if err != nil {
		return errors.Wrap(err, "PolicyCompiler: Error while starting OPA.")
	}

	// Register watcher for rego changes
	(*compConf.ConfigWatcher).Watch(func(changeType watcher.ChangeType, config *configs.ExternalConfig, e error) {
		if changeType == watcher.ChangeRego {
			if err := engine.LoadRegosFromPath(context.Background(), *compConf.RegoDir); err != nil {
				log.Error("PolicyCompiler: Unable to reload regos on file change due to: ", err)
			}
		}
	})

	// Assign variables
	compiler.engine = engine
	compiler.appConfig = appConf
	compiler.config = compConf
	compiler.configured = true
	log.Infoln("Configured PolicyCompiler")
	return nil
}

// See Process() from opa.PolicyCompiler
func (compiler policyCompiler) Process(request *http.Request) (bool, error) {
	if !compiler.configured {
		return false, errors.New("PolicyCompiler was not configured! Please call Configure(). ")
	}

	// Parse body of request
	requestBody := make(map[string]map[string]interface{})
	if log.GetLevel() == log.DebugLevel {
		log.Debugf("PolicyCompiler: Received request: %+v", request)

		// Log body and decode already logged body
		buf := new(bytes.Buffer)
		buf.ReadFrom(request.Body)
		bodyString := buf.String()
		log.Debugf("PolicyCompiler: Request had body: %s", bodyString)
		if marshalErr := json.NewDecoder(strings.NewReader(bodyString)).Decode(&requestBody); marshalErr != nil {
			return false, errors.Wrap(marshalErr, "PolicyCompiler: Error while parsing request body!")
		}
	} else {
		// Decode raw body
		if marshalErr := json.NewDecoder(request.Body).Decode(&requestBody); marshalErr != nil {
			return false, errors.Wrap(marshalErr, "PolicyCompiler: Error while parsing request body!")
		}
	}

	// Extract input
	input := requestBody["input"]
	log.Debugf("PolicyCompiler: Received input: %+v", input)

	// Process path
	output, err := compiler.processPath(input)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during path processing")
	}

	// Compile mapped path
	queries, err := compiler.opaCompile(request, &input, output)
	if err != nil {
		return false, errors.Wrap(err, "PolicyCompiler: Error during policy compilation")
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

func (compiler policyCompiler) processPath(input map[string]interface{}) (*request.PathProcessorOutput, error) {
	inputURL, err := extractURLFromRequestBody(input)
	if err != nil {
		return nil, err
	}
	var method string
	if sentMethod, ok := input["method"]; ok {
		if m, ok := sentMethod.(string); ok {
			method = strings.ToUpper(m)
		} else {
			return nil, errors.Errorf("PolicyCompiler: Attribute 'method' of request body was not of type string! Type was %T", sentMethod)
		}
	} else {
		return nil, errors.New("PolicyCompiler: Request body didn't contain a 'method'. ")
	}

	output, err := (*compiler.config.PathProcessor).Process(&requestInt.URLProcessorInput{
		Method: method,
		URL:    inputURL,
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("Mapped request [%s] to: Datastores [%s] Package: [%s]", inputURL, output.Datastore, output.Package)
	return output, nil
}

func (compiler *policyCompiler) opaCompile(clientRequest *http.Request, input *map[string]interface{}, output *request.PathProcessorOutput) (*rego.PartialQueries, error) {
	// Extract parameters for partial evaluation
	opts := compiler.extractOpaOpts(output)
	extractedInput := extractOpaInput(output, input)
	query := fmt.Sprintf("data.%s.allow == true", output.Package)
	log.Debugf("Sending query=%s", query)

	// Compile clientRequest and return answer
	queries, err := compiler.engine.PartialEvaluate(clientRequest.Context(), extractedInput, query, opts...)
	if err == nil {
		log.Infof("Partial Evaluation for %q with extractedInput: %+vReturned %d queries:", query, extractedInput, len(queries.Queries))
		if log.IsLevelEnabled(log.DebugLevel) {
			for _, q := range queries.Queries {
				log.Debugf("[%+v]", q)
			}
		}
		return queries, nil
	}
	return nil, err
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
			return nil, errors.Wrap(urlError, "PolicyCompiler: Field 'path' from request body is no valid URL")
		case []interface{}:
			stringedURL := make([]string, len(sentURL))
			for i, v := range sentURL {
				stringedURL[i] = fmt.Sprint(v)
			}
			parsed, urlError := url.Parse("/" + strings.Join(stringedURL, "/"))
			if urlError == nil {
				return parsed, nil
			}
			return nil, errors.Wrap(urlError, "PolicyCompiler: Field 'path' from request body is no valid URL")
		default:
			return nil, errors.Errorf("PolicyCompiler: Attribute 'path' of request body was not of type string! Type was %T", sentURL)
		}
	}
	return nil, errors.New("PolicyCompiler: Request body didn't contain a 'path'. ")
}

func (compiler *policyCompiler) extractOpaOpts(output *request.PathProcessorOutput) []func(*rego.Rego) {
	unknowns := []string{fmt.Sprintf("data.%s", output.Datastore)}
	log.Debugf("Sending unknowns %+v", unknowns)
	return []func(*rego.Rego){
		rego.Unknowns(unknowns),
	}
}

func extractOpaInput(output *request.PathProcessorOutput, input *map[string]interface{}) map[string]interface{} {
	extracted := map[string]interface{}{
		"queries": output.Queries,
	}
	// Append custom fields to received body
	for key, value := range *input {
		extracted[key] = value
	}

	// Add parsed extracted without query params
	extracted["path"] = output.Path
	return extracted
}

func initDependencies(compConf *opa.PolicyCompilerConfig, appConf *configs.AppConfig) error {
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

func startOPA(configFile *string, regosPath string) (*OPA, error) {
	ctx := context.Background()
	engine, err := NewOPA(ctx, regosPath, ConfigOPA(*configFile))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize OPA!")
	}

	if err := engine.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "Failed to start OPA!")
	}

	return engine, nil
}
