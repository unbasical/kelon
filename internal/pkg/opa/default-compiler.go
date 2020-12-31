package opa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Foundato/kelon/configs"
	requestInt "github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/pkg/constants/logging"
	internalErrors "github.com/Foundato/kelon/pkg/errors"
	"github.com/Foundato/kelon/pkg/opa"
	"github.com/Foundato/kelon/pkg/request"
	"github.com/Foundato/kelon/pkg/watcher"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type policyCompiler struct {
	configured bool
	appConfig  *configs.AppConfig
	config     *opa.PolicyCompilerConfig
	engine     *OPA
}

type apiResponse struct {
	Result bool `json:"result"`
}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	} `json:"error"`
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

// See Process() from opa.PolicyCompiler
func (compiler policyCompiler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Set start time for request duration
	startTime := time.Now()

	// Validate if policy compiler was configured correctly
	if !compiler.configured {
		compiler.handleError(w, errors.Errorf("PolicyCompiler was not configured! Please call Configure(). "))
		return
	}

	// Parse body of request
	requestBody, bodyErr := compiler.parseRequestBody(req)
	if bodyErr != nil {
		compiler.handleError(w, bodyErr)
		return
	}

	// Extract input
	for rootKey := range requestBody {
		if rootKey != "input" {
			logging.LogForComponent("policyCompiler").Warnf("Request field %q which will be ignored!", rootKey)
		}
	}

	rawInput, exists := requestBody["input"]
	if !exists {
		compiler.handleError(w, internalErrors.InvalidInput{Msg: "PolicyCompiler: Incoming request had no field 'input'!"})
		return
	}
	input, ok := rawInput.(map[string]interface{})
	if !ok {
		compiler.handleError(w, internalErrors.InvalidInput{Msg: "PolicyCompiler: Field 'input' in request body was no nested JSON object!"})
		return
	}
	logging.LogForComponent("policyCompiler").Debugf("Received input: %+v", input)

	// Process path
	output, err := compiler.processPath(input)
	if err != nil {
		compiler.handleError(w, err)
		return
	}

	path, errPath := extractURLFromRequestBody(input)
	method, errMethod := input["method"].(string)
	if errPath != nil || !errMethod {
		compiler.handleError(w, err)
		return
	}

	// Compile mapped path
	queries, err := compiler.opaCompile(req, input, output)
	if err != nil {
		compiler.handleError(w, errors.Wrap(err, "PolicyCompiler: Error during policy compilation"))
		return
	}

	// OPA decided denied
	if queries.Queries == nil {
		compiler.writeDeny(w)
		logging.LogAccessDecision(compiler.config.AccessDecisionLogLevel, path.String(), method, time.Since(startTime).String(), "DENY", "policyCompiler")
		return
	}
	// Check if any query succeeded
	if done := anyQuerySucceeded(queries); done {
		compiler.writeAllow(w)
		logging.LogAccessDecision(compiler.config.AccessDecisionLogLevel, path.String(), method, time.Since(startTime).String(), "ALLOW", "policyCompiler")
		return
	}

	// Otherwise translate ast
	result, err := (*compiler.config.Translator).Process(queries, output.Datastore, req)
	if err != nil {
		compiler.handleError(w, errors.Wrap(err, "PolicyCompiler: Error during ast translation"))
		return
	}

	// If we receive something from the datastore, the query was successful
	if result {
		compiler.writeAllow(w)
		logging.LogAccessDecision(compiler.config.AccessDecisionLogLevel, path.String(), method, time.Since(startTime).String(), "ALLOW", "policyCompiler")
	} else {
		compiler.writeDeny(w)
		logging.LogAccessDecision(compiler.config.AccessDecisionLogLevel, path.String(), method, time.Since(startTime).String(), "DENY", "policyCompiler")
	}
}

func (compiler policyCompiler) handleError(w http.ResponseWriter, err error) {
	log.WithError(err).Error("PolicyCompiler encountered an error")
	// Monitor error
	compiler.handleErrorMetrics(err)

	// Write response
	switch errors.Cause(err).(type) {
	case request.PathAmbiguousError:
		writeError(w, http.StatusNotFound, types.CodeResourceNotFound, err)
	case request.PathNotFoundError:
		writeError(w, http.StatusNotFound, types.CodeResourceNotFound, err)
	case internalErrors.InvalidInput:
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
	default:
		writeError(w, http.StatusInternalServerError, types.CodeInternal, err)
	}
}

func (compiler policyCompiler) handleErrorMetrics(err error) {
	if compiler.appConfig.TelemetryProvider != nil {
		compiler.appConfig.TelemetryProvider.CheckError(err)
	}
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	var resp apiError
	resp.Error.Code = code
	if err != nil {
		resp.Error.Message = errors.Cause(err).Error()
	}
	writeJSON(w, status, resp)
}

func (compiler policyCompiler) writeAllow(w http.ResponseWriter) {
	if compiler.config.RespondWithStatusCode {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSON(w, http.StatusOK, apiResponse{Result: true})
	}
}

func (compiler policyCompiler) writeDeny(w http.ResponseWriter) {
	if compiler.config.RespondWithStatusCode {
		w.WriteHeader(http.StatusForbidden)
	} else {
		writeJSON(w, http.StatusOK, apiResponse{Result: false})
	}
}

func writeJSON(w http.ResponseWriter, status int, x interface{}) {
	bs, _ := json.Marshal(x)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(bs); err != nil {
		logging.LogForComponent("policyCompiler").Fatalln("Unable to send response!")
	}
}

func (compiler policyCompiler) parseRequestBody(req *http.Request) (map[string]interface{}, error) {
	requestBody := make(map[string]interface{})
	if log.GetLevel() == log.DebugLevel {
		logging.LogForComponent("policyCompiler").Debugf("Received request: %+v", req)

		// Log body and decode already logged body
		buf := new(bytes.Buffer)
		if _, parseErr := buf.ReadFrom(req.Body); parseErr != nil {
			return nil, internalErrors.InvalidInput{Cause: parseErr, Msg: "PolicyCompiler: Error while parsing request body!"}
		}

		bodyString := buf.String()
		if bodyString == "" {
			return nil, internalErrors.InvalidInput{Msg: "PolicyCompiler: Request had empty body!"}
		}
		logging.LogForComponent("policyCompiler").Debugf("Request had body: %s", bodyString)
		if marshalErr := json.NewDecoder(strings.NewReader(bodyString)).Decode(&requestBody); marshalErr != nil {
			return nil, internalErrors.InvalidInput{Cause: marshalErr, Msg: "PolicyCompiler: Error while decoding request body!"}
		}
	}

	// Decode raw body
	if marshalErr := json.NewDecoder(req.Body).Decode(&requestBody); marshalErr != nil {
		return nil, internalErrors.InvalidInput{Cause: marshalErr, Msg: "PolicyCompiler: Error while decoding request body!"}
	}
	return requestBody, nil
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
			return nil, internalErrors.InvalidInput{Msg: fmt.Sprintf("PolicyCompiler: Attribute 'method' of request body was not of type string! Type was %T", sentMethod)}
		}
	} else {
		return nil, internalErrors.InvalidInput{Msg: "PolicyCompiler: Object 'input' of request body didn't contain a 'method'"}
	}

	output, err := (*compiler.config.PathProcessor).Process(&requestInt.URLProcessorInput{
		Method: method,
		URL:    inputURL,
	})
	if err != nil {
		return nil, err
	}
	logging.LogForComponent("policyCompiler").Debugf("Mapped request [%s] to: Datastores [%s] Package: [%s]", inputURL, output.Datastore, output.Package)
	return output, nil
}

func (compiler *policyCompiler) opaCompile(clientRequest *http.Request, input map[string]interface{}, output *request.PathProcessorOutput) (*rego.PartialQueries, error) {
	// Extract parameters for partial evaluation
	opts := compiler.extractOpaOpts(output)
	extractedInput := extractOpaInput(output, input)
	query := fmt.Sprintf("data.%s.allow == true", output.Package)
	logging.LogForComponent("policyCompiler").Debugf("Sending query=%s", query)

	// Compile clientRequest and return answer
	queries, err := compiler.engine.PartialEvaluate(clientRequest.Context(), extractedInput, query, opts...)
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
	unknowns := []string{fmt.Sprintf("data.%s", output.Datastore)}
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
