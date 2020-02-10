package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	internalErrors "github.com/Foundato/kelon/pkg/errors"

	utilInt "github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Foundato/kelon/pkg/request"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/open-policy-agent/opa/server/writer"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	} `json:"error"`
}

type apiResponse struct {
	Result bool `json:"result"`
}

type patchImpl struct {
	path  storage.Path
	op    storage.PatchOp
	value interface{}
}

/*
 * ================ Data API ================
 */

func (proxy restProxy) handleV1DataGet(w http.ResponseWriter, r *http.Request) {
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
		log.Warnln("RestProxy: Received GET request without input: " + r.URL.String())
	}

	if trans, err := http.NewRequest("POST", r.URL.String(), strings.NewReader(body)); err == nil {
		// Handle request like post
		proxy.handleV1DataPost(w, trans)
	} else {
		log.Fatal("RestProxy: Unable to map GET request to POST: ", err.Error())
	}
}

func (proxy restProxy) handleV1DataPost(w http.ResponseWriter, r *http.Request) {
	// Add unique identifier for logging purpose
	r = utilInt.AssignRequestUID(r)
	uid := utilInt.GetRequestUID(r)
	log.WithField("UID", uid).Infof("Received OPA Data-API POST to URL: %s", r.RequestURI)

	// Compile
	compiler := *proxy.config.Compiler
	if decision, err := compiler.Process(r); err == nil {
		// Compute status code if configured
		responseStatus := http.StatusOK
		if !decision && proxy.config.RespondWithStatusCode {
			responseStatus = http.StatusForbidden
		}

		// Send decision to client
		switch decision {
		case true:
			log.WithField("UID", uid).Infoln("Decision: ALLOW")
			writeJSON(w, responseStatus, apiResponse{Result: true})
		case false:
			log.WithField("UID", uid).Infoln("Decision: DENY")
			writeJSON(w, responseStatus, apiResponse{Result: false})
		}
	} else {
		// Handle error returned by compiler
		log.WithField("UID", uid).Errorf("RestProxy: Unable to compile request: %s", err.Error())
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
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy restProxy) handleV1DataPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()

	// Parse input
	var value interface{}
	if err := util.NewJSONDecoder(r.Body).Decode(&value); err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
		return
	}

	// Prepare transaction
	path, txn, err := proxy.preparePathCheckedTransaction(ctx, r.URL.Path, opa, w)
	if err != nil {
		return
	}

	// Read from store and check if data already exists
	_, err = opa.Store.Read(ctx, txn, path)
	if err != nil {
		if !storage.IsNotFound(err) {
			proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
			return
		}
		if err := storage.MakeDir(ctx, opa.Store, txn, path[:len(path)-1]); err != nil {
			proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
			return
		}
	} else if r.Header.Get("If-None-Match") == "*" {
		opa.Store.Abort(ctx, txn)
		log.Infof("Update data with If-None-Match header at path: %s", path.String())
		writer.Bytes(w, 304, nil)
		return
	}

	// Write to storage
	if err := opa.Store.Write(ctx, txn, storage.AddOp, path, value); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	proxy.checkPathConflictsCommitAndRespond(ctx, txn, opa, w, path)
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy restProxy) handleV1DataPatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()

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
	txn, err := opa.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Write patches
	for _, patch := range patches {
		// Check path scope before start
		if err := proxy.checkPathScope(ctx, txn, path); err != nil {
			proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
			return
		}

		// Write one patch
		if err := opa.Store.Write(ctx, txn, patch.op, patch.path, patch.value); err != nil {
			proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
			return
		}
	}

	proxy.checkPathConflictsCommitAndRespond(ctx, txn, opa, w, path)
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy restProxy) handleV1DataDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()

	// Prepare transaction
	path, txn, err := proxy.preparePathCheckedTransaction(ctx, r.URL.Path, opa, w)
	if err != nil {
		return
	}

	_, err = opa.Store.Read(ctx, txn, path)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Write to storage
	if err := opa.Store.Write(ctx, txn, storage.RemoveOp, path, nil); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Commit the transaction
	if err := opa.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Write result
	log.Infof("Deleted Data at path: %s", path.String())
	writer.Bytes(w, 204, nil)
}

/*
 * ================ Policy API ================
 */

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy restProxy) handleV1PolicyPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()

	// Read request body
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInvalidParameter, err)
		return
	}

	// Translate
	buf = []byte(utilInt.PreprocessPolicy(proxy.appConf, string(buf)))

	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(r.URL.Path, "/"))
	if !ok {
		writeBadPath(w, r.URL.Path)
		return
	}

	// Start transaction
	txn, err := opa.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Parse module
	parsedMod, err := ast.ParseModule(path.String(), string(buf))
	if err != nil {
		proxy.abortWithBadRequest(ctx, opa, txn, w, err)
		return
	}
	if parsedMod == nil {
		proxy.abortWithBadRequest(ctx, opa, txn, w, errors.New("Empty module"))
		return
	}

	if err = proxy.checkPolicyPackageScope(ctx, txn, parsedMod.Package); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Load all modules and add parsed module
	modules, err := proxy.loadModules(ctx, txn)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}
	modules[path.String()] = parsedMod

	// Compile module in combination with other modules
	c := ast.NewCompiler().SetErrorLimit(1).WithPathConflictsCheck(storage.NonEmpty(ctx, opa.Store, txn))
	if c.Compile(modules); c.Failed() {
		proxy.abortWithBadRequest(ctx, opa, txn, w, c.Errors)
		return
	}

	// Upsert policy
	if err := opa.Store.UpsertPolicy(ctx, txn, path.String(), buf); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Commit the transaction
	if err := opa.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Write result
	log.Infof("Updated Policy at path: %s", path.String())
	writeJSON(w, http.StatusOK, make(map[string]string))
}

// Migration from github.com/open-policy-agent/opa/server/server.go
func (proxy restProxy) handleV1PolicyDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()

	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(r.URL.Path, "/"))
	if !ok {
		writeBadPath(w, r.URL.Path)
		return
	}

	// Start transaction
	txn, err := opa.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return
	}

	// Check policy scope
	if err = proxy.checkPolicyIDScope(ctx, txn, path.String()); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Load all modules and remove module to delete
	modules, err := proxy.loadModules(ctx, txn)
	if err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}
	delete(modules, path.String())

	// Compile module in combination with other modules
	c := ast.NewCompiler().SetErrorLimit(1)
	if c.Compile(modules); c.Failed() {
		proxy.abortWithBadRequest(ctx, opa, txn, w, err)
		return
	}

	// Delete policy
	if err := opa.Store.DeletePolicy(ctx, txn, path.String()); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Commit the transaction
	if err := opa.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}

	// Write result
	log.Infof("Deleted Policy at path: %s", path.String())
	writeJSON(w, http.StatusOK, make(map[string]string))
}

/*
 * ================ Helper Functions ================
 */

func (proxy restProxy) abortWithInternalServerError(ctx context.Context, opa *plugins.Manager, txn storage.Transaction, w http.ResponseWriter, err error) {
	opa.Store.Abort(ctx, txn)
	writeError(w, http.StatusInternalServerError, types.CodeInternal, err)
}

func (proxy restProxy) abortWithBadRequest(ctx context.Context, opa *plugins.Manager, txn storage.Transaction, w http.ResponseWriter, err error) {
	opa.Store.Abort(ctx, txn)
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
		log.Fatalln("RestProxy: Unable to send response!")
	}
}

func (proxy restProxy) loadModules(ctx context.Context, txn storage.Transaction) (map[string]*ast.Module, error) {
	opa := (*proxy.config.Compiler).GetEngine()

	ids, err := opa.Store.ListPolicies(ctx, txn)
	if err != nil {
		return nil, err
	}

	modules := make(map[string]*ast.Module, len(ids))

	for _, id := range ids {
		bs, err := opa.Store.GetPolicy(ctx, txn, id)
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
func (proxy restProxy) prepareV1PatchSlice(root string, ops []types.PatchV1) (result []patchImpl, err error) {
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

func (proxy restProxy) preparePathCheckedTransaction(ctx context.Context, rawPath string, opa *plugins.Manager, w http.ResponseWriter) (storage.Path, storage.Transaction, error) {
	// Parse Path
	path, ok := storage.ParsePathEscaped("/" + strings.Trim(rawPath, "/"))
	if !ok {
		writeBadPath(w, rawPath)
		return nil, nil, errors.New("Error while parsing path")
	}
	// Start transaction
	txn, err := opa.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.CodeInternal, err)
		return nil, nil, err
	}
	// Check path scope before start
	if err := proxy.checkPathScope(ctx, txn, path); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return nil, nil, err
	}
	return path, txn, nil
}

func (proxy restProxy) checkPathConflictsCommitAndRespond(ctx context.Context, txn storage.Transaction, opa *plugins.Manager, w http.ResponseWriter, path storage.Path) {
	// Check path conflicts
	if err := ast.CheckPathConflicts(opa.GetCompiler(), storage.NonEmpty(ctx, opa.Store, txn)); len(err) > 0 {
		proxy.abortWithBadRequest(ctx, opa, txn, w, err)
		return
	}
	// Commit the transaction
	if err := opa.Store.Commit(ctx, txn); err != nil {
		proxy.abortWithInternalServerError(ctx, opa, txn, w, err)
		return
	}
	// Write result
	log.Infof("Created Data at path: %s", path.String())
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

func (proxy restProxy) checkPolicyIDScope(ctx context.Context, txn storage.Transaction, id string) error {
	opa := (*proxy.config.Compiler).GetEngine()

	bs, err := opa.Store.GetPolicy(ctx, txn, id)
	if err != nil {
		return err
	}

	module, err := ast.ParseModule(id, string(bs))
	if err != nil {
		return err
	}

	return proxy.checkPolicyPackageScope(ctx, txn, module.Package)
}

func (proxy restProxy) checkPolicyPackageScope(ctx context.Context, txn storage.Transaction, pkg *ast.Package) error {
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

func (proxy restProxy) checkPathScope(ctx context.Context, txn storage.Transaction, path storage.Path) error {
	opa := (*proxy.config.Compiler).GetEngine()

	names, err := bundle.ReadBundleNamesFromStore(ctx, opa.Store, txn)
	if err != nil {
		if !storage.IsNotFound(err) {
			return err
		}
		return nil
	}

	bundleRoots := map[string][]string{}
	for _, name := range names {
		roots, err := bundle.ReadBundleRootsFromStore(ctx, opa.Store, txn, name)
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
