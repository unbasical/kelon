package monitoring

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

type ApplicationInsights struct {
	AppInsightsInstrumentationKey string
	client                        appinsights.TelemetryClient
}

func (p *ApplicationInsights) Configure() error {
	p.client = appinsights.NewTelemetryClient(os.Getenv("APPINSIGHTS_INSTRUMENTATIONKEY"))
	return nil
}

func (p *ApplicationInsights) GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error) {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			startTime := time.Now()
			passThroughWriter := util.NewPassThroughResponseWriter(writer)
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
