package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/open-policy-agent/opa/server/writer"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	internalErrors "github.com/unbasical/kelon/pkg/errors"
	"github.com/unbasical/kelon/pkg/opa"
	"github.com/unbasical/kelon/pkg/request"
)

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	} `json:"error"`
}

type patchImpl struct {
	path  storage.Path
	op    storage.PatchOp
	value interface{}
}

type decisionContext struct {
	Path           string
	Package        string
	Method         string
	Authentication bool
	Duration       time.Duration
	Error          error
	CorrelationID  uuid.UUID
}

/*
 * ================ Data API ================
 */

func (proxy *restProxy) handleV1DataGet(w http.ResponseWriter, r *http.Request) {
	// Map query parameter "input" to request body
	body := ""
	query := r.URL.Query()
	if keys, ok := query["input"]; ok && len(keys) == 1 {
		// Assign body
		body = keys[0]
		// Remove input query param
		query.Del("input")
		r.URL.RawQuery = query.Encode()
	} else {
		logging.LogForComponent("restProxy").Warnln("Received GET request without input: " + r.URL.String())
	}

	builder := strings.Builder{}
	builder.WriteString(`{"input":`)
	builder.WriteString(body)
	builder.WriteRune('}')

	if trans, err := http.NewRequest("POST", r.URL.String(), strings.NewReader(builder.String())); err == nil {
		// Handle request like post
		proxy.handleV1DataPost(w, trans)
	} else {
		logging.LogForComponent("restProxy").Fatal("Unable to map GET request to POST: ", err.Error())
	}
}

func (proxy *restProxy) handleV1DataPost(w http.ResponseWriter, r *http.Request) {
	// Set start time for request duration
	startTime := time.Now()

	ctx := r.Context()

	// Parse body of request
	requestBody, bodyErr := proxy.parseRequestBody(r)
	if bodyErr != nil {
		proxy.handleError(ctx, w, wrapErrorInLoggingContext(bodyErr))
		return
	}

	decision, err := (*proxy.config.Compiler).Execute(ctx, requestBody)
	duration := time.Since(startTime)

	if err != nil {
		proxy.handleError(ctx, w, wrapErrorInLoggingContext(err))
	}

	if decision.Allow {
		proxy.writeAllow(ctx, w, loggingContextFromDecision(decision, duration))
	} else {
		proxy.writeDeny(ctx, w, loggingContextFromDecision(decision, duration))
	}
}

func (proxy *restProxy) handleV1DataForwardAuth(w http.ResponseWriter, r *http.Request) {
	// Build input body from traefik's forward auth request
	path := r.Header.Get(constants.HeaderXForwardedURI)
	method := r.Header.Get(constants.HeaderXForwardedMethod)

	inputBody := map[string]map[string]interface{}{
		constants.Input: {
			"method": method,
			"path":   path,
		}}

	if r.Header.Get(constants.HeaderAuthorization) != "" {
		inputBody[constants.Input]["token"] = r.Header.Get(constants.HeaderAuthorization)
	}

	body, err := json.Marshal(inputBody)
	if err != nil {
		proxy.handleError(r.Context(), w, wrapErrorInLoggingContext(err))
		return
	}

	endpointData := proxy.pathPrefix + constants.EndpointSuffixData
	if trans, err := http.NewRequest("POST", endpointData, bytes.NewReader(body)); err == nil {
		// Handle request like post
		proxy.handleV1DataPost(w, trans)
	} else {
		logging.LogForComponent("restProxy").Fatal("Unable to map GET request to POST: ", err.Error())
	}
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) handleV1DataPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	engine := (*proxy.config.Compiler).GetEngine()

	// Parse input
	var value interface{}
	if err := util.NewJSONDecoder(r.Body).Decode(&value); err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
		return
	}

	// Prepare transaction
	path, txn, err := proxy.preparePathCheckedTransaction(ctx, r.URL.Path, engine, w)
	if err != nil {
		return
	}
	// Read from store and check if data already exists
	_, err = engine.Store.Read(ctx, txn, path)
	if err != nil {
		if !storage.IsNotFound(err) {
			proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
			return
		}
		if err := storage.MakeDir(ctx, engine.Store, txn, path[:len(path)-1]); err != nil {
			proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
			return
		}
	} else if r.Header.Get("If-None-Match") == "*" {
		engine.Store.Abort(ctx, txn)
		logging.LogForComponent("restProxy").Infof("Update data with If-None-Match header at path: %s", path.String())
		writer.Bytes(w, 304, nil)
		return
	}

	// Write to storage
	if err := engine.Store.Write(ctx, txn, storage.AddOp, path, value); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	proxy.checkPathConflictsCommitAndRespond(ctx, txn, engine, w, path)
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) handleV1DataPatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	engine := (*proxy.config.Compiler).GetEngine()

	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(r.URL.Path, "/"))
	if !ok {
		writeBadPath(w, r.URL.Path)
		return
	}

	// Parse input
	var ops []types.PatchV1
	if err := util.NewJSONDecoder(r.Body).Decode(&ops); err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
		return
	}

	patches, err := proxy.prepareV1PatchSlice(path.String(), ops)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Start transaction
	txn, err := engine.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Write patches
	for _, patch := range patches {
		// Check path scope before start
		if err := proxy.checkPathScope(ctx, txn, path); err != nil {
			proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
			return
		}

		// Write one patch
		if err := engine.Store.Write(ctx, txn, patch.op, patch.path, patch.value); err != nil {
			proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
			return
		}
	}

	proxy.checkPathConflictsCommitAndRespond(ctx, txn, engine, w, path)
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) handleV1DataDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	engine := (*proxy.config.Compiler).GetEngine()

	// Prepare transaction
	path, txn, err := proxy.preparePathCheckedTransaction(ctx, r.URL.Path, engine, w)
	if err != nil {
		return
	}
	_, err = engine.Store.Read(ctx, txn, path)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Write to storage
	if err := engine.Store.Write(ctx, txn, storage.RemoveOp, path, nil); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Commit the transaction
	if err := engine.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Write result
	logging.LogForComponent("restProxy").Infof("Deleted Data at path: %s", path.String())
	writer.Bytes(w, 204, nil)
}

/*
 * ================ Policy API ================
 */

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) handleV1PolicyPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	engine := (*proxy.config.Compiler).GetEngine()

	// Read request body
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
		return
	}

	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(r.URL.Path, "/"))
	if !ok {
		writeBadPath(w, r.URL.Path)
		return
	}

	// Start transaction
	txn, err := engine.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Parse module
	parsedMod, err := ast.ParseModule(path.String(), string(buf))
	if err != nil {
		proxy.abortWithBadRequest(ctx, engine, txn, w, err)
		return
	}
	if parsedMod == nil {
		proxy.abortWithBadRequest(ctx, engine, txn, w, errors.Errorf("empty module"))
		return
	}

	if err = proxy.checkPolicyPackageScope(ctx, txn, parsedMod.Package); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Load all modules and add parsed module
	modules, err := proxy.loadModules(ctx, txn)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}
	modules[path.String()] = parsedMod

	// Compile module in combination with other modules
	c := ast.NewCompiler().SetErrorLimit(1).WithPathConflictsCheck(storage.NonEmpty(ctx, engine.Store, txn))
	c.Compile(modules)
	if c.Failed() {
		proxy.abortWithBadRequest(ctx, engine, txn, w, c.Errors)
		return
	}

	// Upsert policy
	if err := engine.Store.UpsertPolicy(ctx, txn, path.String(), buf); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Commit the transaction
	if err := engine.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Write result
	logging.LogForComponent("restProxy").Debugf("Updated Policy at path: %s", path.String())
	writeJSON(w, http.StatusOK, make(map[string]string))
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) handleV1PolicyDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	engine := (*proxy.config.Compiler).GetEngine()

	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(r.URL.Path, "/"))
	if !ok {
		writeBadPath(w, r.URL.Path)
		return
	}

	// Start transaction
	txn, err := engine.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Check policy scope
	if err = proxy.checkPolicyIDScope(ctx, txn, path.String()); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Load all modules and remove module to delete
	modules, err := proxy.loadModules(ctx, txn)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}
	delete(modules, path.String())

	// Compile module in combination with other modules
	c := ast.NewCompiler().SetErrorLimit(1)
	c.Compile(modules)
	if c.Failed() {
		proxy.abortWithBadRequest(ctx, engine, txn, w, err)
		return
	}

	// Delete policy
	if err := engine.Store.DeletePolicy(ctx, txn, path.String()); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Commit the transaction
	if err := engine.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}

	// Write result
	logging.LogForComponent("restProxy").Infof("Deleted Policy at path: %s", path.String())
	writeJSON(w, http.StatusOK, make(map[string]string))
}

/*
 * ================ Helper Functions ================
 */

func (proxy *restProxy) abortWithInternalServerError(ctx context.Context, engine *plugins.Manager, txn storage.Transaction, w http.ResponseWriter, err error) {
	engine.Store.Abort(ctx, txn)
	writeError(w, http.StatusInternalServerError, types.CodeInternal, err)
}

func (proxy *restProxy) abortWithBadRequest(ctx context.Context, engine *plugins.Manager, txn storage.Transaction, w http.ResponseWriter, err error) {
	engine.Store.Abort(ctx, txn)
	writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	var resp apiError
	resp.Error.Code = code
	if err != nil {
		resp.Error.Message = errors.Cause(err).Error()
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, x interface{}) {
	bs, _ := json.Marshal(x)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(bs); err != nil {
		logging.LogForComponent("restProxy").Fatalln("Unable to send response!")
	}
}

func (proxy *restProxy) loadModules(ctx context.Context, txn storage.Transaction) (map[string]*ast.Module, error) {
	engine := (*proxy.config.Compiler).GetEngine()

	ids, err := engine.Store.ListPolicies(ctx, txn)
	if err != nil {
		return nil, err
	}

	modules := make(map[string]*ast.Module, len(ids))

	for _, id := range ids {
		bs, err := engine.Store.GetPolicy(ctx, txn, id)
		if err != nil {
			return nil, err
		}

		parsed, err := ast.ParseModule(id, string(bs))
		if err != nil {
			return nil, err
		}

		modules[id] = parsed
	}

	return modules, nil
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy *restProxy) prepareV1PatchSlice(root string, ops []types.PatchV1) (result []patchImpl, err error) {
	root = "/" + strings.Trim(root, "/")

	for _, op := range ops {
		impl := patchImpl{value: op.Value}

		// Map patch operation.
		switch op.Op {
		case "add":
			impl.op = storage.AddOp
		case "remove":
			impl.op = storage.RemoveOp
		case "replace":
			impl.op = storage.ReplaceOp
		default:
			return nil, types.BadPatchOperationErr(op.Op)
		}

		// Construct patch path.
		path := strings.Trim(op.Path, "/")
		if len(path) > 0 {
			if root == "/" {
				path = root + path
			} else {
				path = root + "/" + path
			}
		} else {
			path = root
		}

		var ok bool
		impl.path, ok = parsePatchPathEscaped(path)
		if !ok {
			return nil, types.BadPatchPathErr(op.Path)
		}

		result = append(result, impl)
	}

	return result, nil
}

func (proxy *restProxy) preparePathCheckedTransaction(ctx context.Context, rawPath string, engine *plugins.Manager, w http.ResponseWriter) (storage.Path, storage.Transaction, error) {
	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(rawPath, "/"))
	if !ok {
		writeBadPath(w, rawPath)
		return nil, nil, errors.Errorf("Error while parsing path")
	}
	// Start transaction
	txn, err := engine.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return nil, nil, err
	}
	// Check path scope before start
	if err := proxy.checkPathScope(ctx, txn, path); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return nil, nil, err
	}
	return path, txn, nil
}

func (proxy *restProxy) checkPathConflictsCommitAndRespond(ctx context.Context, txn storage.Transaction, engine *plugins.Manager, w http.ResponseWriter, path storage.Path) {
	// Check path conflicts
	if err := ast.CheckPathConflicts(engine.GetCompiler(), storage.NonEmpty(ctx, engine.Store, txn)); len(err) > 0 {
		proxy.abortWithBadRequest(ctx, engine, txn, w, err)
		return
	}
	// Commit the transaction
	if err := engine.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, engine, txn, w, err)
		return
	}
	// Write result
	logging.LogForComponent("restProxy").Infof("Created Data at path: %s", path.String())
	writer.Bytes(w, 204, nil)
}

// Migration from github.com/open-policy-agent/opa/server/server.go
// parsePatchPathEscaped returns a new path for the given escaped str.
// This is based on storage.ParsePathEscaped so will do URL unescaping of
// the provided str for backwards compatibility, but also handles the
// specific escape strings defined in RFC 6901 (JSON Pointer) because
// that's what's mandated by RFC 6902 (JSON Patch).
func parsePatchPathEscaped(str string) (path storage.Path, ok bool) {
	path, ok = storage.ParsePathEscaped(str)
	if !ok {
		return
	}
	for i := range path {
		// RFC 6902 section 4: "[The "path" member's] value is a string containing
		// a JSON-Pointer value [RFC6901] that references a location within the
		// target document (the "target location") where the operation is performed."
		//
		// RFC 6901 section 3: "Because the characters '~' (%x7E) and '/' (%x2F)
		// have special meanings in JSON Pointer, '~' needs to be encoded as '~0'
		// and '/' needs to be encoded as '~1' when these characters appear in a
		// reference token."

		// RFC 6901 section 4: "Evaluation of each reference token begins by
		// decoding any escaped character sequence.  This is performed by first
		// transforming any occurrence of the sequence '~1' to '/', and then
		// transforming any occurrence of the sequence '~0' to '~'.  By performing
		// the substitutions in this order, an implementation avoids the error of
		// turning '~01' first into '~1' and then into '/', which would be
		// incorrect (the string '~01' correctly becomes '~1' after transformation)."
		path[i] = strings.Replace(path[i], "~1", "/", -1)
		path[i] = strings.Replace(path[i], "~0", "~", -1)
	}
	return
}

func (proxy *restProxy) checkPolicyIDScope(ctx context.Context, txn storage.Transaction, id string) error {
	engine := (*proxy.config.Compiler).GetEngine()

	bs, err := engine.Store.GetPolicy(ctx, txn, id)
	if err != nil {
		return err
	}

	module, err := ast.ParseModule(id, string(bs))
	if err != nil {
		return err
	}

	return proxy.checkPolicyPackageScope(ctx, txn, module.Package)
}

func (proxy *restProxy) checkPolicyPackageScope(ctx context.Context, txn storage.Transaction, pkg *ast.Package) error {
	path, err := pkg.Path.Ptr()
	if err != nil {
		return err
	}

	spath, ok := storage.ParsePathEscaped("/" + path)
	if !ok {
		return types.BadRequestErr("invalid package path: cannot determine scope")
	}

	return proxy.checkPathScope(ctx, txn, spath)
}

func (proxy *restProxy) checkPathScope(ctx context.Context, txn storage.Transaction, path storage.Path) error {
	engine := (*proxy.config.Compiler).GetEngine()

	names, err := bundle.ReadBundleNamesFromStore(ctx, engine.Store, txn)
	if err != nil {
		if !storage.IsNotFound(err) {
			return err
		}
		return nil
	}

	bundleRoots := map[string][]string{}
	for _, name := range names {
		roots, err := bundle.ReadBundleRootsFromStore(ctx, engine.Store, txn, name)
		if err != nil && !storage.IsNotFound(err) {
			return err
		}
		bundleRoots[name] = roots
	}

	spath := strings.Trim(path.String(), "/")
	for name, roots := range bundleRoots {
		for _, root := range roots {
			if root != "" && (strings.HasPrefix(spath, root) || strings.HasPrefix(root, spath)) {
				return types.BadRequestErr(fmt.Sprintf("path %v is owned by bundle %q", spath, name))
			}
		}
	}

	return nil
}

func writeBadPath(w http.ResponseWriter, path string) {
	writer.Error(w, http.StatusBadRequest, types.NewErrorV1(types.CodeInvalidParameter, "bad path: %s", path))
}

func (proxy *restProxy) parseRequestBody(req *http.Request) (map[string]interface{}, error) {
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
	} else {
		// Decode raw body
		if marshalErr := json.NewDecoder(req.Body).Decode(&requestBody); marshalErr != nil {
			return nil, internalErrors.InvalidInput{Cause: marshalErr, Msg: "PolicyCompiler: Error while decoding request body!"}
		}
	}
	return requestBody, nil
}

func (proxy *restProxy) handleError(ctx context.Context, w http.ResponseWriter, loggingInfo *decisionContext) {
	logging.LogForComponent("PolicyCompiler").Errorf("Handle error response: %s", loggingInfo.Error)

	// Write response
	switch errors.Cause(loggingInfo.Error).(type) {
	case request.PathAmbiguousError:
		writeError(w, http.StatusNotFound, types.CodeResourceNotFound, loggingInfo.Error)
	case request.PathNotFoundError:
		writeError(w, http.StatusNotFound, types.CodeResourceNotFound, loggingInfo.Error)
	case internalErrors.InvalidInput:
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, loggingInfo.Error)
	case internalErrors.InvalidRequestTranslation:
		proxy.writeDenyError(ctx, w, loggingInfo)
	default:
		writeError(w, http.StatusInternalServerError, types.CodeInternal, loggingInfo.Error)
	}
}

func (proxy *restProxy) writeDenyError(ctx context.Context, w http.ResponseWriter, loggingInfo *decisionContext) {
	proxy.writeDeny(ctx, w, loggingInfo)
	switch err := loggingInfo.Error.(type) {
	case internalErrors.InvalidRequestTranslation:
		for _, e := range err.Causes {
			logging.LogWithCorrelationID(loggingInfo.CorrelationID).Warn(e)
		}
	default:
		logging.LogWithCorrelationID(loggingInfo.CorrelationID).Warn(err.Error())
	}
}

func (proxy *restProxy) writeAllow(ctx context.Context, w http.ResponseWriter, loggingInfo *decisionContext) {
	w.WriteHeader(http.StatusOK)

	labels := map[string]string{
		constants.LabelPolicyDecision: "allow",
		constants.LabelRegoPackage:    loggingInfo.Package,
	}

	proxy.appConf.MetricsProvider.UpdateHistogramMetric(ctx, constants.InstrumentDecisionDuration, loggingInfo.Duration.Milliseconds(), labels)

	logFields := log.Fields{
		logging.LabelPath:     loggingInfo.Path,
		logging.LabelMethod:   loggingInfo.Method,
		logging.LabelDuration: loggingInfo.Duration.String(),
	}

	logging.LogAccessDecision(proxy.config.AccessDecisionLogLevel, "ALLOW", "policyCompiler", logFields)
}

func (proxy *restProxy) writeDeny(ctx context.Context, w http.ResponseWriter, loggingInfo *decisionContext) {
	var reason string
	if !loggingInfo.Authentication {
		reason = "Unauthenticated"
		w.WriteHeader(http.StatusUnauthorized)
	} else {
		reason = "Unauthorized"
		w.WriteHeader(http.StatusForbidden)
	}

	metricLabels := map[string]string{
		constants.LabelPolicyDecision:       "deny",
		constants.LabelPolicyDecisionReason: reason,
		constants.LabelRegoPackage:          loggingInfo.Package,
	}

	proxy.appConf.MetricsProvider.UpdateHistogramMetric(ctx, constants.InstrumentDecisionDuration, loggingInfo.Duration.Milliseconds(), metricLabels)

	logFields := log.Fields{
		logging.LabelPath:     loggingInfo.Path,
		logging.LabelMethod:   loggingInfo.Method,
		logging.LabelDuration: loggingInfo.Duration.String(),
		logging.LabelReason:   reason,
	}

	if loggingInfo.Error != nil {
		logFields[logging.LabelError] = loggingInfo.Error.Error()
		logFields[logging.LabelCorrelation] = loggingInfo.CorrelationID.String()
	}

	logging.LogAccessDecision(proxy.config.AccessDecisionLogLevel, "DENY", "policyCompiler", logFields)
}

func loggingContextFromDecision(decision *opa.Decision, duration time.Duration) *decisionContext {
	return &decisionContext{
		Path:           decision.Path,
		Package:        decision.Package,
		Method:         decision.Method,
		Authentication: decision.Verify,
		Duration:       duration,
	}
}

func wrapErrorInLoggingContext(err error) *decisionContext {
	return &decisionContext{
		Error:         err,
		CorrelationID: uuid.New(),
	}
}
