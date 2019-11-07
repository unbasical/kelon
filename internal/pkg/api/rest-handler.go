package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/storage"

	"github.com/Foundato/kelon/pkg/request"
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

const (
	apiCodeNotFound      = "not_found"
	apiCodeInternalError = "internal_error"
	apiCodeInvalidArgs   = "invalid_args"
)

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
	// Compile
	compiler := *proxy.config.Compiler
	if decision, err := compiler.Process(r); err == nil {
		// Send decision to client
		switch decision {
		case true:
			log.Infoln("Decision: ALLOW")
			writeJSON(w, http.StatusOK, apiResponse{Result: true})
		case false:
			log.Infoln("Decision: DENY")
			writeJSON(w, http.StatusOK, apiResponse{Result: false})
		}
	} else {
		// Handle error returned by compiler
		log.Errorf("RestProxy: Unable to compile request: %s", err.Error())
		switch errors.Cause(err).(type) {
		case *request.PathAmbiguousError:
			writeError(w, http.StatusNotFound, apiCodeNotFound, err)
		default:
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		}
	}
}

/*
 * ================ Policy API ================
 */

func (proxy restProxy) handleV1PolicyPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opa := (*proxy.config.Compiler).GetEngine()
	path := r.URL.Path

	// Read request body
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgs, err)
		return
	}

	// Start transaction
	txn, err := opa.Store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgs, err)
		return
	}

	// Parse module
	parsedMod, err := ast.ParseModule(path, string(buf))
	if err != nil {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgs, err)
		return
	}
	if parsedMod == nil {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgs, errors.New("Empty module"))
		return
	}

	// Load all modules and add parsed module
	modules, err := proxy.loadModules(ctx, txn)
	if err != nil {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}
	modules[path] = parsedMod

	// Compile module in combination with other modules
	c := ast.NewCompiler().SetErrorLimit(1).WithPathConflictsCheck(storage.NonEmpty(ctx, opa.Store, txn))
	if c.Compile(modules); c.Failed() {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgs, c.Errors)
		return
	}

	// Upsert policy
	if err := opa.Store.UpsertPolicy(ctx, txn, path, buf); err != nil {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	// Commit the transaction
	if err := opa.Store.Commit(ctx, txn); err != nil {
		opa.Store.Abort(ctx, txn)
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	// Write result
	log.Infof("Updated Policy at path: %s", path)
	writeJSON(w, http.StatusOK, make(map[string]string))
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
