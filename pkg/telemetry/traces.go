package telemetry

import (
	"context"
	"fmt"
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
	"net/http"
	"strings"
)

type Traces struct {
	provider *sdktrace.TracerProvider
	name     string
}

func NewTraceProvider(ctx context.Context, name string, protocol string, endpoint string) (*Traces, error) {
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

	return &Traces{provider: provider, name: name}, nil
}

func (t *Traces) Configure(ctx context.Context) error {
	logging.LogForComponent("Traces").Infof("Configured Traces")
	return nil
}

func (t *Traces) WrapHTTPHandler(handler http.Handler, spanName string) http.Handler {
	return otelhttp.NewHandler(handler, spanName)
}

func (t *Traces) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	return otelgrpc.UnaryServerInterceptor()
}

func (t *Traces) StartRootSpan(ctx context.Context, spanName string) (context.Context, interface{}) {
	tracer := t.provider.Tracer(t.name)

	return tracer.Start(ctx, spanName, trace.WithNewRoot())
}

func (t *Traces) StartChildSpan(ctx context.Context, spanName string) (context.Context, interface{}) {
	tracer := t.provider.Tracer(t.name)

	return tracer.Start(ctx, spanName)
}

func (t *Traces) RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span == nil || err == nil {
		return
	}

	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

func (t *Traces) Shutdown(ctx context.Context) {
	_ = t.provider.Shutdown(ctx)
}

func newOtlpTraceExporter(ctx context.Context, protocol string, endpoint string) (sdktrace.SpanExporter, error) {
	if endpoint == "" {
		return nil, errors.New("Metric export endpoint must not be empty")
	}

	switch strings.ToLower(protocol) {
	case constants.ProtocolHTTP:
		return otlphttp.New(ctx, otlphttp.WithEndpoint(endpoint), otlphttp.WithInsecure())

	case constants.ProtocolGRPC:
		return otlpgrpc.New(ctx, otlpgrpc.WithEndpoint(endpoint), otlpgrpc.WithInsecure())

	default:
		return nil, errors.New(fmt.Sprintf("Unknown protocol '%s', expected %+v", protocol, []string{constants.ProtocolHTTP, constants.ProtocolGRPC}))
	}
}
