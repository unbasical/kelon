package telemetry

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Foundato/kelon/pkg/constants"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ApplicationInsights struct {
	AppInsightsInstrumentationKey string
	ServiceName                   string
	MaxBatchSize                  int
	MaxBatchIntervalSeconds       int
	client                        appinsights.TelemetryClient
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
	p.client.Context().Tags.Cloud().SetRole(p.ServiceName)
	p.client.Context().Tags.Operation().SetName(p.ServiceName)
	if hostname, err := os.Hostname(); err != nil {
		p.client.Context().Tags.Cloud().SetRoleInstance(hostname)
	}
	log.Infoln("Configured ApplicationInsights.")

	return nil
}

func (p *ApplicationInsights) GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error) {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Monitor method execution
			startTime := time.Now()
			passThroughWriter := NewPassThroughResponseWriter(writer)
			handler.ServeHTTP(passThroughWriter, request)
			duration := time.Since(startTime)
			uid := passThroughWriter.Header().Get(constants.ContextKeyRequestID)

			// Build trace
			trace := appinsights.NewRequestTelemetry(request.Method, request.URL.Path, duration, strconv.Itoa(passThroughWriter.StatusCode()))
			trace.Timestamp = time.Now()
			trace.Source = request.RemoteAddr
			trace.Tags.Operation().SetCorrelationVector(request.Header.Get("correlation-context"))
			parentID := request.Header.Get("request-id")
			trace.Tags.Operation().SetParentId(parentID)
			trace.Tags.Operation().SetId(parentID + uid)
			trace.Properties["user-agent"] = request.Header.Get("User-agent")

			// Send trace
			p.client.Track(trace)
		})
	}, nil
}

func (p *ApplicationInsights) GetHTTPMetricsHandler() (http.Handler, error) {
	return nil, errors.New("Metrics endpoint not supported by ApplicationInsights")
}

func (p *ApplicationInsights) CheckError(err error) {
	if err != nil {
		p.client.TrackException(err)
	}
}

func (p *ApplicationInsights) MeasureRemoteDependency(alias string, dependencyType string, queryTime time.Duration, success bool) {
	dependency := appinsights.RemoteDependencyTelemetry{}
	dependency.Name = alias
	dependency.Type = dependencyType
	dependency.Duration = queryTime
	dependency.Success = success

	// Submit the telemetry
	p.client.Track(&dependency)
}
