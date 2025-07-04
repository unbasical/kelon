package telemetry

import (
	"context"
	"net/http"

	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

type noopMetricsProvider struct{}

// NewNoopMetricProvider instantiates a MetricsProvider which does nothing. This impl is used if no metrics provided is configured
func NewNoopMetricProvider() MetricsProvider {
	logging.LogForComponent("MetricProvider").Info("Metrics not configured")
	return &noopMetricsProvider{}
}

// Configure - see telemetry.MetricsProvider
func (n *noopMetricsProvider) Configure(_ context.Context) error {
	return nil
}

// WrapHTTPHandler - see telemetry.MetricsProvider
func (n *noopMetricsProvider) WrapHTTPHandler(_ context.Context, handler http.Handler) http.Handler {
	return handler
}

// GetHTTPMetricsHandler - see telemetry.MetricsProvider
func (n *noopMetricsProvider) GetHTTPMetricsHandler() (http.Handler, error) {
	return nil, nil
}

// GetGrpcServerInterceptor - see telemetry.MetricsProvider
func (n *noopMetricsProvider) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	}
}

func (n *noopMetricsProvider) GetGrpcInstrumentationHandler() stats.Handler {
	return &noopGrpcStatsHandler{}
}

// UpdateHistogramMetric - see telemetry.MetricsProvider
func (n *noopMetricsProvider) UpdateHistogramMetric(_ context.Context, _ constants.MetricInstrument, _ any, _ map[string]string) {
}

// UpdateGaugeMetric - see telemetry.MetricsProvider
func (n *noopMetricsProvider) UpdateGaugeMetric(_ context.Context, _ constants.MetricInstrument, _ any, _ map[string]string) {
}

// UpdateCounterMetric - see telemetry.MetricsProvider
func (n *noopMetricsProvider) UpdateCounterMetric(_ context.Context, _ constants.MetricInstrument, _ any, _ map[string]string) {
}

// Shutdown - see telemetry.MetricsProvider
func (n *noopMetricsProvider) Shutdown(_ context.Context) {
}

type noopTraceProvider struct{}

// NewNoopTraceProvider instantiates a TraceProvider which does nothing. This impl is used if tracing is not configured.
func NewNoopTraceProvider() TraceProvider {
	logging.LogForComponent("TraceProvider").Info("Tracing not configured")
	return &noopTraceProvider{}
}

// Configure - see telemetry.TraceProvider
func (n *noopTraceProvider) Configure(_ context.Context) error {
	return nil
}

// WrapHTTPHandler - see telemetry.TraceProvider
func (n *noopTraceProvider) WrapHTTPHandler(_ context.Context, handler http.Handler, _ string) http.Handler {
	return handler
}

func (n *noopTraceProvider) GetGrpcInstrumentationHandler() stats.Handler {
	return &noopGrpcStatsHandler{}
}

// ExecuteWithRootSpan - see telemetry.TraceProvider
func (n *noopTraceProvider) ExecuteWithRootSpan(ctx context.Context, function SpanFunction, _ string, _ map[string]string, args ...any) (any, error) {
	return function(ctx, args...)
}

// ExecuteWithChildSpan - see telemetry.TraceProvider
func (n *noopTraceProvider) ExecuteWithChildSpan(ctx context.Context, function SpanFunction, _ string, _ map[string]string, args ...any) (any, error) {
	return function(ctx, args...)
}

// RecordError - see telemetry.TraceProvider
func (n *noopTraceProvider) RecordError(_ context.Context, _ error) {
}

// Shutdown - see telemetry.TraceProvider
func (n *noopTraceProvider) Shutdown(_ context.Context) {
}

// noopGrpcStatsHandler is a no-op implementation of stats.Handler for gRPC
type noopGrpcStatsHandler struct{}

func (n *noopGrpcStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (n *noopGrpcStatsHandler) HandleRPC(_ context.Context, _ stats.RPCStats) {}

func (n *noopGrpcStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (n *noopGrpcStatsHandler) HandleConn(_ context.Context, _ stats.ConnStats) {}
