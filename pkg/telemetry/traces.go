package telemetry

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	otlpgrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlphttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/stats"
)

type traces struct {
	provider *sdktrace.TracerProvider
	name     string
}

// NewTraceProvider tries to create a new trace provider using the provided protocol and push endpoint
func NewTraceProvider(ctx context.Context, name, protocol, endpoint string) (TraceProvider, error) {
	endpointWithoutProtocol := regexp.MustCompile(constants.ProtocolPrefixRe).ReplaceAllString(endpoint, "")
	exporter, err := newOtlpTraceExporter(ctx, protocol, endpointWithoutProtocol)
	if err != nil {
		return nil, err
	}

	defResources := resource.Default()

	r, err := resource.Merge(
		defResources,
		resource.NewWithAttributes(
			defResources.SchemaURL(),
			semconv.ServiceNameKey.String(name),
		),
	)

	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(r))

	otel.SetTracerProvider(provider)

	return &traces{provider: provider, name: name}, nil
}

// Configure - see telemetry.TraceProvider
func (t *traces) Configure(_ context.Context) error {
	logging.LogForComponent("TraceProvider").Info("Tracing configured with exporter of type [otlp]")
	return nil
}

// WrapHTTPHandler - see telemetry.TraceProvider
func (t *traces) WrapHTTPHandler(_ context.Context, handler http.Handler, spanName string) http.Handler {
	return otelhttp.NewHandler(handler, spanName)
}

// GetGrpcInstrumentationHandler - see telemetry.TraceProvider
func (t *traces) GetGrpcInstrumentationHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

// ExecuteWithRootSpan - see telemetry.TraceProvider
func (t *traces) ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...any) (any, error) {
	tracer := t.provider.Tracer(t.name)

	attr := labelsToAttributes(labels)

	ctx, span := tracer.Start(ctx, spanName, trace.WithNewRoot())
	defer span.End()

	span.SetAttributes(attr...)

	ret, err := function(ctx, args...)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}

	return ret, err
}

// ExecuteWithChildSpan - see telemetry.TraceProvider
func (t *traces) ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...any) (any, error) {
	tracer := t.provider.Tracer(t.name)

	attr := labelsToAttributes(labels)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	span.SetAttributes(attr...)

	ret, err := function(ctx, args...)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}

	return ret, err
}

// Shutdown - see telemetry.TraceProvider
func (t *traces) Shutdown(ctx context.Context) {
	_ = t.provider.Shutdown(ctx)
}

func newOtlpTraceExporter(ctx context.Context, protocol, endpoint string) (sdktrace.SpanExporter, error) {
	if endpoint == "" {
		return nil, errors.New("metric export endpoint must not be empty")
	}

	switch strings.ToLower(protocol) {
	case constants.ProtocolHTTP:
		return otlphttp.New(ctx, otlphttp.WithEndpoint(endpoint), otlphttp.WithInsecure())

	case constants.ProtocolGRPC:
		return otlpgrpc.New(ctx, otlpgrpc.WithEndpoint(endpoint), otlpgrpc.WithInsecure())

	default:
		return nil, errors.Errorf("unknown protocol '%s', expected %+v", protocol, []string{constants.ProtocolHTTP, constants.ProtocolGRPC})
	}
}
