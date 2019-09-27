package api

import (
	"encoding/json"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
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
)

func (proxy restProxy) handleGet(w http.ResponseWriter, r *http.Request) {
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
		proxy.handlePost(w, trans)
	} else {
		log.Fatal("RestProxy: Unable to map GET request to POST: ", err.Error())
	}
}

func (proxy restProxy) handlePost(w http.ResponseWriter, r *http.Request) {
	// Compile
	compiler := *proxy.config.Compiler
	if decision, err := compiler.Process(r); err == nil {
		// Send decision to client
		switch decision {
		case true:
			writeJSON(w, http.StatusOK, apiResponse{Result: true})
		case false:
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
