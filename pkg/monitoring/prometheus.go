package monitoring

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

type Prometheus struct{}

func (p Prometheus) GetGrpcStreamInterceptor() (*grpc.StreamServerInterceptor, error) {
	panic("implement me")
}

func (p Prometheus) GetGrpcUnaryInterceptor() (*grpc.UnaryServerInterceptor, error) {
	panic("implement me")
}

func (p Prometheus) RegisterHTTPMiddleware() (func(handler http.Handler) http.HandlerFunc, error) {
	panic("implement me")
}

func (p Prometheus) GetHTTPHandler() (http.Handler, error) {
	return promhttp.Handler(), nil
}
