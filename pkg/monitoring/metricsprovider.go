package monitoring

import (
	"net/http"

	"google.golang.org/grpc"
)

type MetricsProvider interface {
	// Configure metrics provider
	Configure() error
	// Get a Grpc Stream Interceptor
	GetGrpcStreamInterceptor() (*grpc.StreamServerInterceptor, error)
	// Get a Grpc Unary Interceptor
	GetGrpcUnaryInterceptor() (*grpc.UnaryServerInterceptor, error)
	// Get a func which wraps a http Handler as Middleware
	GetHTTPMiddleware() (func(handler http.Handler) http.Handler, error)
	// Get a handler which can be exposed as "/metrics" endpoint
	GetHTTPMetricsHandler() (http.Handler, error)
	// Check errors for additional metrics
	CheckError(err error)
}
