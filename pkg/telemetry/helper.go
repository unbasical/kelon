package telemetry

import (
	"net/http"

	"github.com/pkg/errors"
)

func ApplyTelemetryIfPresent(provider Provider, handler http.Handler) (http.Handler, error) {
	if provider != nil {
		telemetryMiddleware, middErr := provider.GetHTTPMiddleware()
		if middErr != nil {
			return nil, errors.Wrap(middErr, "TelemetryProvider does not implement 'GetHTTPMiddleware()' correctly.")
		}
		return telemetryMiddleware(handler), nil
	}
	return handler, nil
}
