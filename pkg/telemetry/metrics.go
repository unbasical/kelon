package telemetry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unbasical/kelon/common"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	otelrun "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	otlpgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlphttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/unit"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc"
)

type Metrics struct {
	provider         *sdkmetric.MeterProvider
	name             string
	exportType       string
	instrumentsSync  map[constants.MetricInstrument]instrument.Synchronous
	instrumentsAsync map[constants.MetricInstrument]instrument.Asynchronous
}

const ErrorInstrumentNotFound string = "instrument with name %s not found"

// New creates a new Metrics struct exporting metrics using the specified format and the protocol to use
// If the Prometheus format is chosen, the protocol attribute will be ignored
func New(ctx context.Context, name string, format string, protocol string) (*Metrics, error) {
	m := &Metrics{
		name:             name,
		instrumentsSync:  make(map[constants.MetricInstrument]instrument.Synchronous),
		instrumentsAsync: make(map[constants.MetricInstrument]instrument.Asynchronous),
	}

	switch strings.ToLower(format) {
	case constants.TelemetryPrometheus:
		exporter, err := prometheus.New()
		if err != nil {
			return nil, err
		}
		m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
		m.exportType = constants.TelemetryPrometheus
	case constants.TelemetryOtlp:
		exporter, err := newOtlpExporter(ctx, protocol)
		if err != nil {
			return nil, err
		}
		m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
		m.exportType = constants.TelemetryOtlp
	default:
		return nil, errors.New(fmt.Sprintf("Unknown format '%s', expected one of %+v", format, []string{constants.TelemetryPrometheus, constants.TelemetryOtlp}))
	}

	global.SetMeterProvider(m.provider)

	return m, nil
}

// Configure the Metrics instance and default metrics:
// - HTTP
//   - Active Requests - Gauge
//   - Request Duration - Histogram
//   - Request Size - Histogram
//
// - GRPC
//   - Request Duration - Histogram
//   - Request Size - Histogram
//
// - Version - Gauge (won't change on runtime)
// If no provider is set, the default provider (Prometheus) will be used
// Please note that Configure has to be called once before the component can be used (Otherwise Metric calls will return an error)
func (m *Metrics) Configure(ctx context.Context) error {
	if m.provider == nil {
		exporter, err := prometheus.New()
		if err == nil {
			return err
		}
		m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	}

	if err := m.initSpecMetrics(); err != nil {
		return err
	}

	if err := m.initCustomMetrics(); err != nil {
		return err
	}

	// Instrument Version Metric
	m.instrumentVersion(ctx)

	// Instrument Runtime
	if err := otelrun.Start(otelrun.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		return err
	}

	logging.LogForComponent("Metrics").Infof("Configured Metrics with %s Exporter.", m.exportType)

	return nil
}

func (m *Metrics) GetHTTPMiddleware(ctx context.Context) (func(handler http.Handler) http.Handler, error) {
	return func(handler http.Handler) http.Handler {
		return m.instrumentHandlerActiveRequest(ctx,
			m.instrumentHandlerDuration(ctx,
				m.instrumentHandlerRequestSize(ctx, handler)))
	}, nil
}

func (m *Metrics) GetHTTPMetricsHandler() (http.Handler, error) {
	if m.exportType == constants.TelemetryPrometheus {
		return promhttp.Handler(), nil
	} else {
		return nil, nil
	}
}

func (m *Metrics) WriteMetricDecision(ctx context.Context, decision Decision) {
	decisionDuration, ok := m.instrumentsSync[constants.InstrumentDecisionDuration].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentDecisionDuration.String())
		return
	}

	attr := []attribute.KeyValue{
		attribute.Key(constants.LabelPolicyDecision).String(decision.PolicyDecision),
		attribute.Key(constants.LabelRegoPackage).String(decision.Package),
	}

	decisionDuration.Record(ctx, decision.Duration, attr...)
}

func (m *Metrics) WriteMetricQuery(ctx context.Context, dbQuery DbQuery) {
	queryDuration, ok := m.instrumentsSync[constants.InstrumentDbQueryDuration].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentDbQueryDuration.String())
		return
	}

	attr := []attribute.KeyValue{
		attribute.Key(constants.LabelDbPoolName).String(dbQuery.PoolName),
		attribute.Key(constants.LabelRegoPackage).String(dbQuery.Package),
	}

	queryDuration.Record(ctx, dbQuery.Duration, attr...)
}

func (m *Metrics) CheckError(err error) {
	// not needed in prometheus
}

func (m *Metrics) Shutdown(ctx context.Context) {
	_ = m.provider.Shutdown(ctx)
}

func (m *Metrics) ExportType() string {
	return m.exportType
}

// GetGrpcServerMetricInterceptor Interceptor to gather rpc metrics
func (m *Metrics) GetGrpcServerMetricInterceptor() grpc.UnaryServerInterceptor {
	fallback := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}

	requestDuration, ok := m.instrumentsSync[constants.InstrumentRpcRequestDuration].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentRpcRequestDuration.String())
		return fallback
	}

	requestSize, ok := m.instrumentsSync[constants.InstrumentRpcRequestSize].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentRpcRequestSize.String())
		return fallback
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		reqSize, err := approximateGrpcRequestSize(req)
		if err != nil {
			logging.LogForComponent("GetGrpcServerMetricInterceptor").Error("Error determining the request size", err)
		}

		attrs := []attribute.KeyValue{
			attribute.Key(constants.LabelGrpcService).String(info.FullMethod),
		}

		now := time.Now()
		resp, err := handler(ctx, req)

		duration := time.Since(now).Milliseconds()

		requestDuration.Record(ctx, duration, attrs...)
		requestSize.Record(ctx, int64(reqSize))

		return resp, err
	}
}

// Initialize Instruments according to with OpenTelemetry Spec
func (m *Metrics) initSpecMetrics() error {
	meter := m.provider.Meter(m.name)

	httpActiveRequests, err := meter.SyncInt64().UpDownCounter(
		constants.InstrumentHttpActiveRequests.String(),
		instrument.WithUnit("{requests}"),
		instrument.WithDescription("A gauge of requests currently being served by the wrapped handler."))
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentHttpActiveRequests] = httpActiveRequests

	httpRequestDuration, err := meter.SyncInt64().Histogram(
		constants.InstrumentHttpRequestDuration.String(),
		instrument.WithUnit(unit.Milliseconds),
		instrument.WithDescription("A histogram of latencies for requests."),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentHttpRequestDuration] = httpRequestDuration

	httpRequestSize, err := meter.SyncInt64().Histogram(
		constants.InstrumentHttpRequestSize.String(),
		instrument.WithUnit(unit.Bytes),
		instrument.WithDescription("A histogram of request sizes."),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentHttpRequestSize] = httpRequestSize

	rpcRequestSize, err := meter.SyncInt64().Histogram(
		constants.InstrumentRpcRequestSize.String(),
		instrument.WithUnit(unit.Bytes),
		instrument.WithDescription("A histogram of request sizes."),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentRpcRequestSize] = rpcRequestSize

	rpcRequestDuration, err := meter.SyncInt64().Histogram(
		constants.InstrumentRpcRequestDuration.String(),
		instrument.WithUnit(unit.Milliseconds),
		instrument.WithDescription("A histogram of latencies for requests."),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentRpcRequestDuration] = rpcRequestDuration

	version, err := meter.SyncInt64().UpDownCounter(
		constants.InstrumentVersion.String(),
		instrument.WithUnit("{version}"),
		instrument.WithDescription("Version information about this binary"),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentVersion] = version

	return nil
}

// Initialize Instruments for custom use cases
func (m *Metrics) initCustomMetrics() error {
	meter := m.provider.Meter(m.name)

	decisionDuration, err := meter.SyncInt64().Histogram(
		constants.InstrumentDecisionDuration.String(),
		instrument.WithUnit(unit.Milliseconds),
		instrument.WithDescription("A histogram of latencies for decisions."),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentDecisionDuration] = decisionDuration

	dbQueryDuration, err := meter.SyncInt64().Histogram(
		constants.InstrumentDbQueryDuration.String(),
		instrument.WithUnit(unit.Milliseconds),
		instrument.WithDescription("A histogram of latencies for db queries"),
	)
	if err != nil {
		return err
	}
	m.instrumentsSync[constants.InstrumentDbQueryDuration] = dbQueryDuration

	return nil
}

// Active Request Metric
func (m *Metrics) instrumentHandlerActiveRequest(ctx context.Context, next http.Handler) http.Handler {
	httpActiveRequests, ok := m.instrumentsSync[constants.InstrumentHttpActiveRequests].(syncint64.UpDownCounter)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHttpActiveRequests.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpActiveRequests.Add(ctx, 1)
		next.ServeHTTP(w, r)
		httpActiveRequests.Add(ctx, -1)
	})
}

// Request Duration Metric
func (m *Metrics) instrumentHandlerDuration(ctx context.Context, next http.Handler) http.Handler {
	httpRequestDuration, ok := m.instrumentsSync[constants.InstrumentHttpRequestDuration].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHttpRequestDuration.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passthrough := NewPassThroughResponseWriter(w)

		now := time.Now()

		next.ServeHTTP(passthrough, r)

		duration := time.Since(now).Milliseconds()
		attrs := []attribute.KeyValue{
			attribute.Key(constants.LabelHttpMethod).String(r.Method),
			attribute.Key(constants.LabelHttpStatusCode).Int(passthrough.statusCode),
		}

		httpRequestDuration.Record(ctx, duration, attrs...)
	})
}

// Request Size Metric
func (m *Metrics) instrumentHandlerRequestSize(ctx context.Context, next http.Handler) http.Handler {
	httpRequestSize, ok := m.instrumentsSync[constants.InstrumentHttpRequestSize].(syncint64.Histogram)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHttpRequestSize.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size := int64(approximateHttpRequestSize(r))
		httpRequestSize.Record(ctx, size)
		next.ServeHTTP(w, r)
	})
}

// Version Metric (won't change on runtime)
func (m *Metrics) instrumentVersion(ctx context.Context) {
	version, ok := m.instrumentsSync[constants.InstrumentHttpRequestSize].(syncint64.UpDownCounter)
	if !ok {
		logging.LogForComponent("Metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHttpRequestSize)
		return
	}

	version.Add(ctx, 0, attribute.Key("version").String(common.Version))
}

// create new OTLP metric Exporter
func newOtlpExporter(ctx context.Context, protocol string) (sdkmetric.Exporter, error) {
	otelEndpoint, ok := os.LookupEnv(constants.EnvOtlpEndpoint)
	if !ok {
		return nil, errors.New(fmt.Sprintf("Environment Variable %s not found", constants.EnvOtlpEndpoint))
	}

	switch strings.ToLower(protocol) {
	case constants.ProtocolHttp:
		return otlphttp.New(ctx, otlphttp.WithEndpoint(otelEndpoint), otlphttp.WithInsecure())

	case constants.ProtocolGrpc:
		return otlpgrpc.New(ctx, otlpgrpc.WithEndpoint(otelEndpoint), otlpgrpc.WithInsecure())

	default:
		return nil, errors.New(fmt.Sprintf("Unknown protocol '%s', expected %+v", protocol, []string{constants.ProtocolHttp, constants.ProtocolGrpc}))
	}
}

func approximateHttpRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s += len(r.URL.String())
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}

func approximateGrpcRequestSize(req interface{}) (int, error) {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(req)
	if err != nil {
		logging.LogForComponent("Metrics").Errorf("encode error: %+v", err)
		return 0, err
	}
	return binary.Size(buff.Bytes()), nil
}
