package telemetry

import (
	"context"
	"net/http"

	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"google.golang.org/grpc"
)

type noopMetricsProvider struct{}

func NewNoopMetricProvider() MetricsProvider {
	logging.LogForComponent("MetricProvider").Info("Metrics not configured")
	return &noopMetricsProvider{}
}

func (n *noopMetricsProvider) Configure(ctx context.Context) error {
	return nil
}

func (n *noopMetricsProvider) WrapHTTPHandler(ctx context.Context, handler http.Handler) http.Handler {
	return handler
}

func (n *noopMetricsProvider) GetHTTPMetricsHandler() (http.Handler, error) {
	return nil, nil
}

func (n *noopMetricsProvider) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

func (n *noopMetricsProvider) UpdateHistogramMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *noopMetricsProvider) UpdateGaugeMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *noopMetricsProvider) UpdateCounterMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *noopMetricsProvider) Shutdown(ctx context.Context) {
}

type noopTraceProvider struct{}

func NewNoopTraceProvider() TraceProvider {
	logging.LogForComponent("TraceProvider").Info("Tracing not configured")
	return &noopTraceProvider{}
}

func (n *noopTraceProvider) Configure(ctx context.Context) error {
	return nil
}

func (n *noopTraceProvider) WrapHTTPHandler(ctx context.Context, handler http.Handler, spanName string) http.Handler {
	return handler
}

func (n *noopTraceProvider) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

func (n *noopTraceProvider) ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
	return function(ctx, args...)
}

func (n *noopTraceProvider) ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
	return function(ctx, args...)
}

func (n *noopTraceProvider) RecordError(ctx context.Context, err error) {
}

func (n *noopTraceProvider) Shutdown(ctx context.Context) {
}
