package telemetry

import (
	"net/http"
)

type Provider interface {
	// Configure telemetry provider
	Configure() error
	// Get a func which wraps a http Handler as Middleware
	GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error)
	// Get a handler which can be exposed as "/metrics" endpoint
	GetHTTPMetricsHandler() (http.Handler, error)
	// Check errors for additional metrics
	CheckError(err error)
	// Gracefully shutdown
	Shutdown()
}
