package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ApplicationInsights struct {
	AppInsightsInstrumentationKey string
	client                        appinsights.TelemetryClient
	MaxBatchSize                  int
	MaxBatchIntervalSeconds       int
}

func (p *ApplicationInsights) Configure() error {
	if p.AppInsightsInstrumentationKey == "" {
		return errors.New("ApplicationInsights: No Instrumentation-Key was provided before configuration!")
	}
	telemetryConfig := appinsights.NewTelemetryConfiguration(p.AppInsightsInstrumentationKey)
	// Configure how many items can be sent in one call to the data collector:
	telemetryConfig.MaxBatchSize = p.MaxBatchSize
	// Configure the maximum delay before sending queued telemetry:
	telemetryConfig.MaxBatchInterval = time.Second * time.Duration(p.MaxBatchIntervalSeconds)

	p.client = appinsights.NewTelemetryClientFromConfig(telemetryConfig)
	log.Infoln("Configured ApplicationInsights.")

	return nil
}

func (p *ApplicationInsights) GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error) {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			startTime := time.Now()
			passThroughWriter := NewPassThroughResponseWriter(writer)
			handler.ServeHTTP(passThroughWriter, request)
			duration := time.Since(startTime)
			trace := appinsights.NewRequestTelemetry(request.Method, request.URL.Path, duration, strconv.Itoa(passThroughWriter.StatusCode()))
			trace.Timestamp = time.Now()
			p.client.Track(trace)
		})
	}, nil
}

func (p *ApplicationInsights) GetHTTPMetricsHandler() (http.Handler, error) {
	return nil, nil
}

func (p *ApplicationInsights) CheckError(err error) {
	if err != nil {
		trace := appinsights.NewTraceTelemetry(err.Error(), appinsights.Error)
		trace.Timestamp = time.Now()
		p.client.Track(trace)
	}
}
