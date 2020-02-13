package telemetry

import (
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Foundato/kelon/pkg/constants"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	log "github.com/sirupsen/logrus"
)

type ApplicationInsights struct {
	AppInsightsInstrumentationKey string
	ServiceName                   string
	MaxBatchSize                  int
	MaxBatchIntervalSeconds       int
	LogLevels                     string
	StatsIntervalSeconds          int
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

	// Log diagnostic data to logger
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Infof("ApplicationInsights Diagnostics: %s\n", msg)
		return nil
	})

	// Send Logs
	if p.LogLevels != "" {
		// Process args and initialize logger
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: false,
		})
		log.AddHook(p)
	}

	// Start reporting system metrics
	go p.TrackStats()

	log.Infoln("Configured ApplicationInsights.")
	return nil
}

// Log levels for logrus hook
func (p *ApplicationInsights) Levels() []log.Level {
	var logLevels []log.Level
	for _, levelName := range strings.Split(p.LogLevels, ",") {
		level, err := log.ParseLevel(strings.TrimSpace(levelName))
		if err != nil {
			log.Warnf("ApplicationInsights: Unable to handle input-log-level %s. It will be skipped! Error is: %s", level, err.Error())
		}
		logLevels = append(logLevels, level)
	}
	return logLevels
}

// Capture log event from logrus hook
func (p *ApplicationInsights) Fire(entry *log.Entry) error {
	trace := appinsights.TraceTelemetry{}
	if msg, err := entry.String(); err == nil {
		trace.Message = msg
	} else {
		trace.Message = entry.Message
	}
	trace.SetTime(entry.Time)
	switch entry.Level {
	case log.FatalLevel:
		trace.SeverityLevel = appinsights.Critical
	case log.PanicLevel:
		trace.SeverityLevel = appinsights.Critical
	case log.ErrorLevel:
		trace.SeverityLevel = appinsights.Error
	case log.WarnLevel:
		trace.SeverityLevel = appinsights.Warning
	case log.InfoLevel:
		trace.SeverityLevel = appinsights.Information
	case log.DebugLevel:
		trace.SeverityLevel = appinsights.Verbose
	case log.TraceLevel:
		trace.SeverityLevel = appinsights.Verbose
	}
	p.client.Track(&trace)
	return nil
}

func (p *ApplicationInsights) TrackStats() {
	quit := make(chan os.Signal, 1) // buffered
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		select {
		case <-quit:
			log.Println("Application Insights: Stopped")
			return
		case <-time.After(time.Second * time.Duration(p.StatsIntervalSeconds)):
			// Track Memory stats
			if virtMem, err := mem.VirtualMemory(); err == nil {
				p.client.TrackMetric("Heap Memory Used", float64(virtMem.Used))
				p.client.TrackMetric("% Heap Memory Used", virtMem.UsedPercent)
			} else {
				p.CheckError(err)
			}

			// Track CPU stats
			if cpuPercentages, err := cpu.Percent(0, false); err == nil {
				for _, cpuPercentage := range cpuPercentages {
					p.client.TrackMetric("% Processor Time", cpuPercentage)
				}
			} else {
				p.CheckError(err)
			}

			// Track Network stats
			if netIOStats, err := net.IOCounters(false); err == nil {
				for _, netIOStat := range netIOStats {
					p.client.TrackMetric("IO Data Bytes/sec", float64(netIOStat.BytesRecv+netIOStat.BytesSent)/5)
					p.client.TrackMetric("Data In-Bytes/sec", float64(netIOStat.BytesRecv)/5)
					p.client.TrackMetric("Data Out-Bytes/sec", float64(netIOStat.BytesSent)/5)
				}
			} else {
				p.CheckError(err)
			}
		}
	}
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

func (p *ApplicationInsights) MeasureRemoteDependency(name string, dependencyType string, queryTime time.Duration, data string, success bool) {
	dependency := appinsights.RemoteDependencyTelemetry{}
	dependency.Name = name
	dependency.Type = dependencyType
	dependency.Duration = queryTime
	dependency.Data = data
	dependency.Success = success

	// Submit the telemetry
	p.client.Track(&dependency)
}

func (p *ApplicationInsights) Shutdown() {
	p.client.TrackAvailability("Internal", time.Duration(0), false)
	select {
	case <-p.client.Channel().Close(10 * time.Second):
		// Ten second timeout for retries.

		// If we got here, then all telemetry was submitted
		// successfully, and we can proceed to exiting.
	case <-time.After(30 * time.Second):
		// Thirty second absolute timeout.  This covers any
		// previous telemetry submission that may not have
		// completed before Close was called.

		// There are a number of reasons we could have
		// reached here.  We gave it a go, but telemetry
		// submission failed somewhere.  Perhaps old events
		// were still retrying, or perhaps we're throttled.
		// Either way, we don't want to wait around for it
		// to complete, so let's just exit.
	}
}
