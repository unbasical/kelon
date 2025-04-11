package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/telemetry"
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

// applyHandlerMiddleware applies monitoring middlewares and additionally configured middlewares like e.g. inputHeaderMappingMiddleware
func (proxy *restProxy) applyHandlerMiddleware(ctx context.Context, endpoint string, handlerFunc http.HandlerFunc, options ...middlewareOption) http.Handler {
	ops := defaultMiddlewareOptions()
	for _, option := range options {
		option(ops)
	}

	var wrappedHandler http.Handler = handlerFunc

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		wrappedHandler = proxy.applyLoggingMiddleware(wrappedHandler)
	}

	if ops.headerExtraction {
		wrappedHandler = proxy.inputHeaderMappingMiddleware(wrappedHandler)
	}

	wrappedHandler = proxy.appConf.MetricsProvider.WrapHTTPHandler(ctx, wrappedHandler)
	wrappedHandler = proxy.appConf.TraceProvider.WrapHTTPHandler(ctx, wrappedHandler, endpoint)

	return wrappedHandler
}

// inputHeaderMappingMiddleware tries to extract values from either the url query (for GET method) or the body and
// adds available values to the request header before calling the next http.Handler.
// The values which will be extracted can be configured via configs.Global
func (proxy *restProxy) inputHeaderMappingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
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

// applyLoggingMiddleware wraps the provided http.Handler with basic logging containing in addition to url and method
// the request header, response status and the handling duration
func (proxy *restProxy) applyLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		pw := telemetry.NewPassThroughResponseWriter(w)
		next.ServeHTTP(pw, r)

		duration := time.Since(start)
		logging.LogForComponent("restProxy").
			WithField("method", r.Method).
			WithField("url", r.URL.Path).
			WithField("headers", r.Header).
			WithField("status", pw.StatusCode()).
			WithField("duration", duration.String()).
			Debug("Request processed")
	})
}

// extractInputFromQuery tries to JSON parse the value for the url query parameter constants.Input
func extractInputFromQuery(r *http.Request) (map[string]any, error) {
	body := map[string]any{constants.Input: make(map[string]any)}
	query := r.URL.Query()
	if keys, ok := query[constants.Input]; ok && len(keys) == 1 {
		// Assign body
		var value map[string]any
		if err := json.Unmarshal([]byte(keys[0]), &value); err != nil {
			return nil, errors.Wrapf(err, "Unable to parse input")
		}
		body[constants.Input] = value
	}
	return body, nil
}

// extractInputFromBody tries to JSON parse the request body
func extractInputFromBody(r *http.Request) (map[string]any, error) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, errors.Wrap(err, "Unable to parse input")
	}

	return body, nil
}

// writeInputToQuery writes the provided body as JSON to the http.Request url as query value for key constants.Input.
// values other than constants.Input are ignored.
func writeInputToQuery(r *http.Request, body map[string]any) error {
	value, err := json.Marshal(body[constants.Input])
	if err != nil {
		return errors.Wrap(err, "Unable to marshal input")
	}

	query := r.URL.Query()
	query.Set(constants.Input, string(value))
	r.URL.RawQuery = query.Encode()

	return nil
}

// writeInputToBody writes the provided data as JSON to the request body
func writeInputToBody(r *http.Request, body map[string]any) error {
	enrichedMarshalled, err := json.Marshal(body)
	if err != nil {
		return errors.Wrap(err, "Unable to marshal input")
	}

	r.Body = io.NopCloser(bytes.NewBuffer(enrichedMarshalled))
	return nil
}

// applyHeaderMappingsToInput inserts the Header (specified by the config) into the input
func (proxy *restProxy) applyHeaderMappingsToInput(body map[string]any, r *http.Request) (map[string]any, error) {
	var input map[string]any
	value, ok := body[constants.Input]
	if !ok {
		// No input provided, create empty input
		input = make(map[string]any)
	} else if input, ok = value.(map[string]any); !ok {
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
