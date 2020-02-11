package constants

// Telemetry provider which is supported by kelon.
type TelemetryProvider string

// TelemetryProvider for Prometheus
const PrometheusTelemetry TelemetryProvider = "prometheus"

// TelemetryProvider for ApplicationInsights
const ApplicationInsightsTelemetry TelemetryProvider = "applicationinsights"
