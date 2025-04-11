package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
)

// labelsToAttributes converts a label map to OTel []attribute.KeyValue
func labelsToAttributes(labels map[string]string) []attribute.KeyValue {
	var attr []attribute.KeyValue

	for key, val := range labels {
		attr = append(attr, attribute.Key(key).String(val))
	}

	return attr
}
