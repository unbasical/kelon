package telemetry

import (
	"context"
	"google.golang.org/grpc"
	"net/http"
)

type Decision struct {
	Package        string
	Duration       int64
	PolicyDecision string
}

type DbQuery struct {
	Package  string
	Duration int64
	PoolName string
}

type MetricsProvider interface {
	// Configure telemetry provider
	Configure(ctx context.Context) error
	// GetHTTPMiddleware Get a func which wraps a http Handler as Middleware
	GetHTTPMiddleware(ctx context.Context) (func(handler http.Handler) http.Handler, error)
	// GetHTTPMetricsHandler Get a handler which can be exposed as "/metrics" endpoint
	GetHTTPMetricsHandler() (http.Handler, error)
	// GetGrpcServerInterceptor Get an interceptor to instrument grpc calls
	GetGrpcServerInterceptor() grpc.UnaryServerInterceptor
	// WriteMetricDecision updates the decision metric
	WriteMetricDecision(ctx context.Context, decision Decision)
	// WriteMetricQuery updates the query metric
	WriteMetricQuery(ctx context.Context, query DbQuery)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
	// ExportType return the metric exporter used
	ExportType() string
}

type TraceProvider interface {
	Configure(ctx context.Context) error
	// WrapHTTPHandler Wrap a HTTP handler with span creation
	WrapHTTPHandler(handler http.Handler, spanName string) http.Handler
	// GetGrpcServerInterceptor Get an interceptor to instrument grpc calls
	GetGrpcServerInterceptor() grpc.UnaryServerInterceptor
	// StartRootSpan starts a new span as a root span
	StartRootSpan(ctx context.Context, spanName string) (context.Context, interface{})
	// StartChildSpan opens a child span in the current context
	StartChildSpan(ctx context.Context, spanName string) (context.Context, interface{})
	// RecordError records the error on the current span located in context
	// If no span found in context or err is nil -> do nothing
	RecordError(ctx context.Context, err error)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
}
