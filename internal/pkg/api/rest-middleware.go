package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
)

type middlewareOption = func(options *middlewareOptions)

type middlewareOptions struct {
	headerExtraction bool
}

func defaultMiddlewareOptions() *middlewareOptions {
	return &middlewareOptions{
		headerExtraction: false,
	}
}

func withHeaderExtraction(enable bool) middlewareOption {
	return func(options *middlewareOptions) {
		options.headerExtraction = enable
	}
}

func (proxy *restProxy) applyHandlerMiddleware(ctx context.Context, endpoint string, handlerFunc http.HandlerFunc, options ...middlewareOption) http.Handler {
	ops := defaultMiddlewareOptions()
	for _, option := range options {
		option(ops)
	}

	var wrappedHandler http.Handler = handlerFunc

	if ops.headerExtraction {
		wrappedHandler = proxy.inputHeaderMappingMiddleware(wrappedHandler)
	}

	wrappedHandler = proxy.appConf.MetricsProvider.WrapHTTPHandler(ctx, wrappedHandler)
	wrappedHandler = proxy.appConf.TraceProvider.WrapHTTPHandler(ctx, wrappedHandler, endpoint)

	return wrappedHandler
}

func (proxy *restProxy) inputHeaderMappingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		var err error

		if r.Method == http.MethodGet {
			// Get the input from query for GET requests
			body, err = extractInputFromQuery(r)
		} else {
			// Get from request body for all other requests
			body, err = extractInputFromBody(r)
		}
		if err != nil {
			logging.LogForComponent("restProxy").Errorf("Unable to extract input: %s", err.Error())
			http.Error(w, "Unable to extract input", http.StatusBadRequest)
			return
		}

		// Enrich input with header mappings
		enriched, err := proxy.applyHeaderMappingsToInput(body, r)
		if err != nil {
			logging.LogForComponent("restProxy").Errorf("Unable to enrich input: %s", err.Error())
			http.Error(w, "Unable to enrich input", http.StatusInternalServerError)
			return
		}

		// Write back to request
		if r.Method == http.MethodGet {
			err = writeInputToQuery(r, enriched)
		} else {
			err = writeInputToBody(r, enriched)
		}
		if err != nil {
			logging.LogForComponent("restProxy").Errorf("Unable to write input: %s", err.Error())
			http.Error(w, "Unable to write input", http.StatusInternalServerError)
			return
		}

		// Add header mapping here
		next.ServeHTTP(w, r)
	})
}

func extractInputFromQuery(r *http.Request) (map[string]interface{}, error) {
	body := map[string]interface{}{constants.Input: make(map[string]interface{})}
	query := r.URL.Query()
	if keys, ok := query[constants.Input]; ok && len(keys) == 1 {
		// Assign body
		var value map[string]interface{}
		if err := json.Unmarshal([]byte(keys[0]), &value); err != nil {
			return nil, errors.Wrapf(err, "Unable to parse input")
		}
		body[constants.Input] = value
	}
	return body, nil
}

func extractInputFromBody(r *http.Request) (map[string]interface{}, error) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, errors.Wrap(err, "Unable to parse input")
	}

	return body, nil
}

func writeInputToQuery(r *http.Request, body map[string]interface{}) error {
	value, err := json.Marshal(body[constants.Input])
	if err != nil {
		return errors.Wrap(err, "Unable to marshal input")
	}

	query := r.URL.Query()
	query.Set(constants.Input, string(value))
	r.URL.RawQuery = query.Encode()

	return nil
}

func writeInputToBody(r *http.Request, body map[string]interface{}) error {
	enrichedMarshalled, err := json.Marshal(body)
	if err != nil {
		return errors.Wrap(err, "Unable to marshal input")
	}

	r.Body = io.NopCloser(bytes.NewBuffer(enrichedMarshalled))
	return nil
}

// applyHeaderMappingsToInput inserts the Header (specified by the config) into the input
func (proxy *restProxy) applyHeaderMappingsToInput(body map[string]interface{}, r *http.Request) (map[string]interface{}, error) {
	var input map[string]interface{}
	value, ok := body[constants.Input]
	if !ok {
		// No input provided, create empty input
		input = make(map[string]interface{})
	} else if input, ok = value.(map[string]interface{}); !ok {
		return nil, errors.Errorf("Mismatched type for body[%s]. Expected %T but got %T", constants.Input, input, value)
	}

	for _, mapping := range proxy.appConf.Global.Input.HeaderMapping {
		headerValue := r.Header.Get(mapping.Name)
		if headerValue == "" {
			continue
		}
		input[mapping.Alias] = headerValue
	}

	body[constants.Input] = input
	return body, nil
}
