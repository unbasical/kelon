package telemetry

import (
	"context"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"net/http"

	"github.com/unbasical/kelon/pkg/constants"
	"google.golang.org/grpc"
)

type NoopMetricsProvider struct{}

func NewNoopMetricProvider() MetricsProvider {
	logging.LogForComponent("MetricProvider").Info("Metrics not configured")
	return &NoopMetricsProvider{}
}

func (n *NoopMetricsProvider) Configure(ctx context.Context) error {
	return nil
}

func (n *NoopMetricsProvider) WrapHTTPHandler(ctx context.Context, handler http.Handler) http.Handler {
	return handler
}

func (n *NoopMetricsProvider) GetHTTPMetricsHandler() (http.Handler, error) {
	return nil, nil
}

func (n *NoopMetricsProvider) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

func (n *NoopMetricsProvider) UpdateHistogramMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *NoopMetricsProvider) UpdateGaugeMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *NoopMetricsProvider) UpdateCounterMetric(ctx context.Context, metric constants.MetricInstrument, value interface{}, labels map[string]string) {
}

func (n *NoopMetricsProvider) Shutdown(ctx context.Context) {
}

type NoopTraceProvider struct{}

func NewNoopTraceProvider() TraceProvider {
	logging.LogForComponent("TraceProvider").Info("Tracing not configured")
	return &NoopTraceProvider{}
}

func (n *NoopTraceProvider) Configure(ctx context.Context) error {
	return nil
}

func (n *NoopTraceProvider) WrapHTTPHandler(ctx context.Context, handler http.Handler, spanName string) http.Handler {
	return handler
}

func (n *NoopTraceProvider) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

func (n *NoopTraceProvider) ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
	return function(ctx, args...)
}

func (n *NoopTraceProvider) ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
	return function(ctx, args...)
}

func (n *NoopTraceProvider) RecordError(ctx context.Context, err error) {
}

func (n *NoopTraceProvider) Shutdown(ctx context.Context) {
}
