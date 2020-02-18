package telemetry

import (
	"net/http"
	"time"

	"github.com/Foundato/kelon/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type Prometheus struct {
	registry *prometheus.Registry
}

// nolint:gochecknoglobals
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
	errorsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "errors_total",
		Help: "A gauge of non-fatal errors.",
	})
	databaseRequestsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "datastore_requests_total",
		Help: "Count of all Datastore requests",
	})
	databaseErrorsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "datastore_errors_total",
		Help: "A gauge of non-fatal errors during datastore requests errors.",
	})
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
		[]string{"handler", "method"},
	)
	datastoreRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "datastore_request_duration_seconds",
			Help:    "A histogram of latencies for datastore requests.",
			Buckets: []float64{.05, .1, .5, 1, 2.5, 10},
		},
		[]string{"database"},
	)
	// responseSize has no labels, making it a zero-dimensional
	// ObserverVec.
	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_size_bytes",
			Help:    "A histogram of request sizes.",
			Buckets: []float64{100, 400, 900, 1500},
		},
		[]string{},
	)
)

func (p *Prometheus) Configure() error {
	if p.registry == nil {
		p.registry = prometheus.NewRegistry()
		// System stats
		p.registry.MustRegister(version, prometheus.NewGoCollector(), prometheus.NewBuildInfoCollector())
		// Http
		p.registry.MustRegister(httpRequestsTotal, inFlightRequests, duration, requestSize, errorsCount)
		// Datastores
		p.registry.MustRegister(databaseRequestsTotal, datastoreRequestDuration, databaseErrorsCount)

		log.Infoln("Configured Prometheus.")
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
	errorsCount.Inc()
}

func (p *Prometheus) MeasureRemoteDependency(request *http.Request, alias string, dependencyType string, queryTime time.Duration, data string, success bool) {
	databaseRequestsTotal.Inc()
	if success {
		datastoreRequestDuration.With(prometheus.Labels{"database": alias}).Observe(queryTime.Seconds())
	} else {
		databaseErrorsCount.Inc()
	}
}

func (p *Prometheus) Shutdown() {}
