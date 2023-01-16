package telemetry

import (
	"context"
	"net/http"
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
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type traces struct {
	provider *sdktrace.TracerProvider
	name     string
}

func NewOtlpTraceProvider(ctx context.Context, name, protocol, endpoint string) (TraceProvider, error) {
	exporter, err := newOtlpTraceExporter(ctx, protocol, endpoint)
	if err != nil {
		return nil, err
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
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

func (t *traces) Configure(ctx context.Context) error {
	logging.LogForComponent("TraceProvider").Info("Tracing configured with exporter of type [otlp]")
	return nil
}

func (t *traces) WrapHTTPHandler(ctx context.Context, handler http.Handler, spanName string) http.Handler {
	return otelhttp.NewHandler(handler, spanName)
}

func (t *traces) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return otelgrpc.UnaryServerInterceptor()
}

func (t *traces) ExecuteWithRootSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
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

func (t *traces) ExecuteWithChildSpan(ctx context.Context, function SpanFunction, spanName string, labels map[string]string, args ...interface{}) (interface{}, error) {
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
