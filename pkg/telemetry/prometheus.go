package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unbasical/kelon/common"
	"github.com/unbasical/kelon/pkg/constants/logging"
)

type Prometheus struct {
	registry *prometheus.Registry
}

// nolint:gochecknoglobals,gocritic
var (
	version = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "version",
		Help: "Version information about this binary",
		ConstLabels: map[string]string{
			"version": common.Version,
		},
	})
	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Count of all HTTP requests",
	}, []string{"code", "method"})
	inFlightRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "in_flight_requests",
		Help: "A gauge of requests currently being served by the wrapped handler.",
	})
	// duration is partitioned by the HTTP method and handler. It uses custom
	// buckets based on the expected request duration.
	duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_duration_seconds",
			Help:    "A histogram of latencies for requests.",
			Buckets: []float64{.05, .1, .5, 1, 2.5, 10},
		},
		[]string{"handler", "method", "code"},
	)
	// responseSize has no labels, making it a zero-dimensional
	// ObserverVec.
	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_size_bytes",
			Help:    "A histogram of request sizes.",
			Buckets: []float64{100, 400, 900, 1500},
		},
		[]string{"code", "method"},
	)
)

func (p *Prometheus) Configure() error {
	if p.registry == nil {
		p.registry = prometheus.NewRegistry()
		// System stats
		p.registry.MustRegister(version, collectors.NewGoCollector(), collectors.NewBuildInfoCollector())
		// Http
		p.registry.MustRegister(httpRequestsTotal, inFlightRequests, duration, requestSize)

		logging.LogForComponent("Prometheus").Infoln("Configured Prometheus.")
	}
	return nil
}

func (p *Prometheus) GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error) {
	return func(handler http.Handler) http.Handler {
		return promhttp.InstrumentHandlerInFlight(inFlightRequests,
			promhttp.InstrumentHandlerDuration(duration.MustCurryWith(prometheus.Labels{"handler": "http"}),
				promhttp.InstrumentHandlerCounter(httpRequestsTotal,
					promhttp.InstrumentHandlerRequestSize(requestSize, handler),
				),
			),
		)
	}, nil
}

func (p *Prometheus) GetHTTPMetricsHandler() (http.Handler, error) {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}), nil
}

func (p *Prometheus) CheckError(err error) {
	// not needed in prometheus
}

func (p *Prometheus) Shutdown() {}
