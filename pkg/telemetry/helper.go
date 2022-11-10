package telemetry

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
)

func ApplyTelemetryIfPresent(ctx context.Context, provider MetricsProvider, handler http.Handler) (http.Handler, error) {
	if provider != nil {
		telemetryMiddleware, middErr := provider.GetHTTPMiddleware(ctx)
		if middErr != nil {
			return nil, errors.Wrap(middErr, "MetricsProvider does not implement 'GetHTTPMiddleware()' correctly.")
		}
		return telemetryMiddleware(handler), nil
	}
	return handler, nil
}
