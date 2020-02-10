package monitoring

import (
	"net/http"

	"google.golang.org/grpc"
)

type MetricsProvider interface {
	GetGrpcStreamInterceptor() (*grpc.StreamServerInterceptor, error)
	GetGrpcUnaryInterceptor() (*grpc.UnaryServerInterceptor, error)
	RegisterHTTPMiddleware() (func(handler http.Handler) http.HandlerFunc, error)
	GetHTTPHandler() (http.Handler, error)
}
