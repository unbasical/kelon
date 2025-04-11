package telemetry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unbasical/kelon/common"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	otelrun "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlpgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlphttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc"
)

type metrics struct {
	provider    *sdkmetric.MeterProvider
	name        string
	exportType  string
	instruments map[constants.MetricInstrument]any
}

const (
	// ErrorInstrumentNotFound is the error msg template which will be used for invalid instrument IDs on updates
	ErrorInstrumentNotFound string = "instrument with name %s not found"
	// ErrorValueNotCastable is the error msg template which will be used if an instrument is found, but it is
	// of a different type than the expected one.
	ErrorValueNotCastable string = "unable to cast to %T: %+v"

	// UnitMilliseconds is the unit label for milliseconds
	UnitMilliseconds = "ms"
	// UnitBytes is the unit label for bytes
	UnitBytes = "By"
)

// NewMetricsProvider creates a new metrics struct exporting metrics using the specified format and the protocol to use
// If the Prometheus format is chosen, the protocol attribute will be ignored
func NewMetricsProvider(ctx context.Context, name, format, protocol, endpoint string) (MetricsProvider, error) {
	m := &metrics{
		name:        name,
		instruments: make(map[constants.MetricInstrument]any),
	}

	endpointWithoutProtocol := regexp.MustCompile(constants.ProtocolPrefixRe).ReplaceAllString(endpoint, "")
	switch strings.ToLower(format) {
	case constants.TelemetryPrometheus:
		exporter, err := prometheus.New()
		if err != nil {
			return nil, err
		}
		m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
		m.exportType = constants.TelemetryPrometheus
	case constants.TelemetryOtlp:
		exporter, err := newOtlpMetricExporter(ctx, protocol, endpointWithoutProtocol)
		if err != nil {
			return nil, err
		}
		m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
		m.exportType = constants.TelemetryOtlp
	default:
		return nil, errors.Errorf("unknown format '%s', expected one of %+v", format, []string{constants.TelemetryPrometheus, constants.TelemetryOtlp})
	}

	otel.SetMeterProvider(m.provider)

	return m, nil
}

// Configure the metrics instance and default metrics:
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
func (m *metrics) Configure(ctx context.Context) error {
	if m.provider == nil {
		exporter, err := prometheus.New()
		if err != nil {
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

	logging.LogForComponent("MetricProvider").Infof("metrics configured with exporter of type [%s]", m.exportType)
	return nil
}

// WrapHTTPHandler - see telemetry.MetricsProvider
func (m *metrics) WrapHTTPHandler(ctx context.Context, handler http.Handler) http.Handler {
	return m.instrumentHandlerActiveRequest(ctx,
		m.instrumentHandlerDuration(ctx,
			m.instrumentHandlerRequestSize(ctx, handler)))
}

// GetHTTPMetricsHandler - see telemetry.MetricsProvider
func (m *metrics) GetHTTPMetricsHandler() (http.Handler, error) {
	if m.exportType == constants.TelemetryPrometheus {
		return promhttp.Handler(), nil
	}
	return nil, nil
}

// UpdateHistogramMetric - see telemetry.MetricsProvider
func (m *metrics) UpdateHistogramMetric(ctx context.Context, instrument constants.MetricInstrument, value any, labels map[string]string) {
	histogram, ok := m.instruments[instrument].(metric.Int64Histogram)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, instrument.String())
		return
	}

	val, ok := value.(int64)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorValueNotCastable, int64(0), value)
	}

	attr := labelsToAttributes(labels)

	histogram.Record(ctx, val, metric.WithAttributes(attr...))
}

// UpdateGaugeMetric - see telemetry.MetricsProvider
func (m *metrics) UpdateGaugeMetric(ctx context.Context, instrument constants.MetricInstrument, value any, labels map[string]string) {
	gauge, ok := m.instruments[instrument].(metric.Int64UpDownCounter)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, instrument.String())
		return
	}

	val, ok := value.(int64)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorValueNotCastable, int64(0), value)
	}

	attr := labelsToAttributes(labels)

	gauge.Add(ctx, val, metric.WithAttributes(attr...))
}

// UpdateCounterMetric - see telemetry.MetricsProvider
func (m *metrics) UpdateCounterMetric(ctx context.Context, instrument constants.MetricInstrument, value any, labels map[string]string) {
	counter, ok := m.instruments[instrument].(metric.Int64Counter)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, instrument.String())
		return
	}

	val, ok := value.(int64)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorValueNotCastable, int64(0), value)
	}

	attr := labelsToAttributes(labels)

	counter.Add(ctx, val, metric.WithAttributes(attr...))
}

// Shutdown - see telemetry.MetricsProvider
func (m *metrics) Shutdown(ctx context.Context) {
	_ = m.provider.Shutdown(ctx)
}

// GetGrpcServerInterceptor Interceptor to gather rpc metrics
func (m *metrics) GetGrpcServerInterceptor() grpc.UnaryServerInterceptor {
	fallback := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	}

	requestDuration, ok := m.instruments[constants.InstrumentRPCRequestDuration].(metric.Int64Histogram)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentRPCRequestDuration.String())
		return fallback
	}

	requestSize, ok := m.instruments[constants.InstrumentRPCRequestSize].(metric.Int64Histogram)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentRPCRequestSize.String())
		return fallback
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		reqSize, err := approximateGrpcRequestSize(req)
		if err != nil {
			logging.LogForComponent("GetGrpcServerInterceptor").Error("Error determining the request size", err)
		}

		attrs := []attribute.KeyValue{
			attribute.Key(constants.LabelGrpcService).String(info.FullMethod),
		}

		now := time.Now()
		resp, err := handler(ctx, req)

		duration := time.Since(now).Milliseconds()

		requestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
		requestSize.Record(ctx, int64(reqSize))

		return resp, err
	}
}

// Initialize Instruments according to with OpenTelemetry Spec
func (m *metrics) initSpecMetrics() error {
	meter := m.provider.Meter(m.name)

	httpActiveRequests, err := meter.Int64UpDownCounter(
		constants.InstrumentHTTPActiveRequests.String(),
		metric.WithUnit("{requests}"),
		metric.WithDescription("A gauge of requests currently being served by the wrapped handler."))
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentHTTPActiveRequests] = httpActiveRequests

	httpRequestDuration, err := meter.Int64Histogram(
		constants.InstrumentHTTPRequestDuration.String(),
		metric.WithUnit(UnitMilliseconds),
		metric.WithDescription("A histogram of latencies for requests."),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentHTTPRequestDuration] = httpRequestDuration

	httpRequestSize, err := meter.Int64Histogram(
		constants.InstrumentHTTPRequestSize.String(),
		metric.WithUnit(UnitBytes),
		metric.WithDescription("A histogram of request sizes."),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentHTTPRequestSize] = httpRequestSize

	rpcRequestSize, err := meter.Int64Histogram(
		constants.InstrumentRPCRequestSize.String(),
		metric.WithUnit(UnitBytes),
		metric.WithDescription("A histogram of request sizes."),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentRPCRequestSize] = rpcRequestSize

	rpcRequestDuration, err := meter.Int64Histogram(
		constants.InstrumentRPCRequestDuration.String(),
		metric.WithUnit(UnitMilliseconds),
		metric.WithDescription("A histogram of latencies for requests."),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentRPCRequestDuration] = rpcRequestDuration

	version, err := meter.Int64UpDownCounter(
		constants.InstrumentVersion.String(),
		metric.WithUnit("{version}"),
		metric.WithDescription("Version information about this binary"),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentVersion] = version

	return nil
}

// Initialize Instruments for custom use cases
func (m *metrics) initCustomMetrics() error {
	meter := m.provider.Meter(m.name)

	decisionDuration, err := meter.Int64Histogram(
		constants.InstrumentDecisionDuration.String(),
		metric.WithUnit(UnitMilliseconds),
		metric.WithDescription("A histogram of latencies for decisions."),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentDecisionDuration] = decisionDuration

	dbQueryDuration, err := meter.Int64Histogram(
		constants.InstrumentDBQueryDuration.String(),
		metric.WithUnit(UnitMilliseconds),
		metric.WithDescription("A histogram of latencies for db queries"),
	)
	if err != nil {
		return err
	}
	m.instruments[constants.InstrumentDBQueryDuration] = dbQueryDuration

	return nil
}

// Active Request Metric
func (m *metrics) instrumentHandlerActiveRequest(ctx context.Context, next http.Handler) http.Handler {
	httpActiveRequests, ok := m.instruments[constants.InstrumentHTTPActiveRequests].(metric.Int64UpDownCounter)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHTTPActiveRequests.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpActiveRequests.Add(ctx, 1)
		next.ServeHTTP(w, r)
		httpActiveRequests.Add(ctx, -1)
	})
}

// Request Duration Metric
func (m *metrics) instrumentHandlerDuration(ctx context.Context, next http.Handler) http.Handler {
	httpRequestDuration, ok := m.instruments[constants.InstrumentHTTPRequestDuration].(metric.Int64Histogram)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHTTPRequestDuration.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passthrough := NewPassThroughResponseWriter(w)

		now := time.Now()

		next.ServeHTTP(passthrough, r)

		duration := time.Since(now).Milliseconds()
		attrs := []attribute.KeyValue{
			attribute.Key(constants.LabelHTTPMethod).String(r.Method),
			attribute.Key(constants.LabelHTTPStatusCode).Int(passthrough.statusCode),
		}

		httpRequestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	})
}

// Request Size Metric
func (m *metrics) instrumentHandlerRequestSize(ctx context.Context, next http.Handler) http.Handler {
	httpRequestSize, ok := m.instruments[constants.InstrumentHTTPRequestSize].(metric.Int64Histogram)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHTTPRequestSize.String())
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size := int64(approximateHTTPRequestSize(r))
		httpRequestSize.Record(ctx, size)
		next.ServeHTTP(w, r)
	})
}

// Version Metric (won't change on runtime)
func (m *metrics) instrumentVersion(ctx context.Context) {
	version, ok := m.instruments[constants.InstrumentHTTPRequestSize].(metric.Int64UpDownCounter)
	if !ok {
		logging.LogForComponent("metrics").Errorf(ErrorInstrumentNotFound, constants.InstrumentHTTPRequestSize)
		return
	}

	version.Add(ctx, 0, metric.WithAttributes(attribute.Key("version").String(common.Version)))
}

// create new OTLP metric Exporter
func newOtlpMetricExporter(ctx context.Context, protocol, endpoint string) (sdkmetric.Exporter, error) {
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

func approximateHTTPRequestSize(r *http.Request) int {
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

func approximateGrpcRequestSize(req any) (int, error) {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(req)
	if err != nil {
		logging.LogForComponent("metrics").Errorf("encode error: %+v", err)
		return 0, err
	}
	return binary.Size(buff.Bytes()), nil
}
