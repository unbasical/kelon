package telemetry

import (
	"context"
	"net/http"

	"github.com/unbasical/kelon/pkg/constants"
	"google.golang.org/grpc"
)

type SpanFunction func(ctx context.Context, args ...interface{}) (interface{}, error)

type MetricsProvider interface {
	// Configure metrics provider
	Configure(ctx context.Context) error
	// WrapHTTPHandler Wrap an HTTP handler with metrics
	WrapHTTPHandler(ctx context.Context, handler http.Handler) http.Handler
	// GetHTTPMetricsHandler Get a handler which can be exposed as "/metrics" endpoint
	GetHTTPMetricsHandler() (http.Handler, error)
	// GetGrpcServerInterceptor Get an interceptor to instrument grpc calls
	GetGrpcServerInterceptor() grpc.UnaryServerInterceptor
	// UpdateHistogramMetric write a value with labels to a histogram
	UpdateHistogramMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string)
	// UpdateGaugeMetric write a value with labels to a gauge
	UpdateGaugeMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string)
	// UpdateCounterMetric Increment a counter by a value
	UpdateCounterMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
}

type TraceProvider interface {
	// Configure trace provider
	Configure(ctx context.Context) error
	// WrapHTTPHandler Wrap an HTTP handler with span creation
	WrapHTTPHandler(ctx context.Context, handler http.Handler, spanName string) http.Handler
	// GetGrpcServerInterceptor Get an interceptor to instrument grpc calls
	GetGrpcServerInterceptor() grpc.UnaryServerInterceptor
	// ExecuteWithRootSpan starts a new root span and executes the function
	ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error)
	// ExecuteWithChildSpan starts a new child span and executes the function
	ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error)
	// Shutdown Gracefully shutdown
	Shutdown(ctx context.Context)
}
