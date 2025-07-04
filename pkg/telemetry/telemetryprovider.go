package telemetry

import (
	"context"
	"net/http"

	"github.com/unbasical/kelon/pkg/constants"
	"google.golang.org/grpc/stats"
)

// SpanFunction represents a function, which will be wrapped in a tracing span
type SpanFunction func(ctx context.Context, args ...any) (any, error)

// MetricsProvider is able to record and publish metrics
type MetricsProvider interface {
	// Configure metrics provider
	Configure(ctx context.Context) error
	// WrapHTTPHandler Wrap an HTTP handler with metrics
	WrapHTTPHandler(ctx context.Context, handler http.Handler) http.Handler
	// GetHTTPMetricsHandler Get a handler which can be exposed as "/metrics" endpoint
	GetHTTPMetricsHandler() (http.Handler, error)
	// GetGrpcInstrumentationHandler returns the instrumentation handler for a gRPC server
	GetGrpcInstrumentationHandler() stats.Handler
	// UpdateHistogramMetric write a value with labels to a histogram
	UpdateHistogramMetric(ctx context.Context, metric constants.MetricInstrument, value any, labels map[string]string)
	// UpdateGaugeMetric write a value with labels to a gauge
	UpdateGaugeMetric(ctx context.Context, metric constants.MetricInstrument, value any, labels map[string]string)
	// UpdateCounterMetric Increment a counter by a value
	UpdateCounterMetric(ctx context.Context, metric constants.MetricInstrument, value any, labels map[string]string)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
}

// TraceProvider is able to record and publish traces
type TraceProvider interface {
	// Configure trace provider
	Configure(ctx context.Context) error
	// WrapHTTPHandler Wrap an HTTP handler with span creation
	WrapHTTPHandler(ctx context.Context, handler http.Handler, spanName string) http.Handler
	// GetGrpcInstrumentationHandler returns the instrumentation handler for a gRPC server
	GetGrpcInstrumentationHandler() stats.Handler
	// ExecuteWithRootSpan starts a new root span and executes the function
	ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...any) (any, error)
	// ExecuteWithChildSpan starts a new child span and executes the function
	ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...any) (any, error)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
}
