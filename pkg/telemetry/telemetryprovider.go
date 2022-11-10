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
	// GetGrpcServerMetricInterceptor Get an interceptor to instrument grpc calls
	GetGrpcServerMetricInterceptor() grpc.UnaryServerInterceptor
	// WriteMetricDecision updates the decision metric
	WriteMetricDecision(ctx context.Context, decision Decision)
	WriteMetricQuery(ctx context.Context, query DbQuery)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
	// ExportType return the metric exporter used
	ExportType() string
}
