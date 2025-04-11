package constants

// ContextKey represents keys for key value pairs in contexts
type ContextKey string

// ContextKeyRegoPackage is used to propagate the rego package via the context in order to enrich logs/spans
const ContextKeyRegoPackage = ContextKey("regoPackage")

// HTTP Request related constants
const (
	// Input is the attribute in a JSON request body, which contains the necessary data for policy evaluation
	Input = "input"
	// EndpointData is used for all data related http endpoints
	EndpointData = "/data"
	// EndpointPolicies is used for all policy related http endpoints
	EndpointPolicies = "/policies"
	// EndpointHealth is used as the http endpoint for liveliness probes
	EndpointHealth = "/health"
	// EndpointMetrics will be used if Kelon is configured to publish metrics using Prometheus
	EndpointMetrics = "/metrics"
	// URLParamID is the url parameter which will be used by http endpoints, which try to query/modify e.g. policies
	URLParamID = "id"
)

// Datastore related constants
const (
	// MetaMaxOpenConnections is the MetaKey for maxOpenConnections
	MetaMaxOpenConnections string = "maxOpenConnections"
	// MetaMaxIdleConnections is the MetaKey for maxIdleConnections
	MetaMaxIdleConnections string = "maxIdleConnections"
	// MetaConnectionMaxLifetimeSeconds is the MetaKey for connectionMaxLifetimeSeconds
	MetaConnectionMaxLifetimeSeconds string = "connectionMaxLifetimeSeconds"
)

// Telemetry Configuration
const (
	// TelemetryPrometheus is the telemetry option for prometheus
	TelemetryPrometheus string = "prometheus"
	// TelemetryOtlp is the telemetry option for OpenTelemetry
	TelemetryOtlp string = "otlp"
	// ProtocolHTTP indicates HTTP should be used for exporting OpenTelemetry data
	ProtocolHTTP string = "http"
	// ProtocolGRPC indicates gRPC should be used for exporting OpenTelemetry data
	ProtocolGRPC string = "grpc"
)

// MetricInstrument identifies the metric instruments
type MetricInstrument int

const (
	// InstrumentVersion represents the version metric
	InstrumentVersion MetricInstrument = iota
	// InstrumentHTTPRequestDuration represents the http request duration metric
	InstrumentHTTPRequestDuration
	// InstrumentHTTPActiveRequests represents the http active request metric
	InstrumentHTTPActiveRequests
	// InstrumentHTTPRequestSize represents the http request size metric
	InstrumentHTTPRequestSize
	// InstrumentRPCRequestDuration represents the rpc request metric
	InstrumentRPCRequestDuration
	// InstrumentRPCRequestSize represents the rpc request size metric
	InstrumentRPCRequestSize
	// InstrumentDecisionDuration represents the decision duration metric
	InstrumentDecisionDuration
	// InstrumentDBQueryDuration represents the database query duration metric
	InstrumentDBQueryDuration
)

func (i MetricInstrument) String() string {
	switch i {
	case InstrumentVersion:
		return "version"
	case InstrumentHTTPRequestDuration:
		return "http.server.duration"
	case InstrumentHTTPActiveRequests:
		return "http.server.active_requests"
	case InstrumentHTTPRequestSize:
		return "http.server.request.size"
	case InstrumentRPCRequestDuration:
		return "rpc.server.duration"
	case InstrumentRPCRequestSize:
		return "rpc.server.request.size"
	case InstrumentDecisionDuration:
		return "decision.duration"
	case InstrumentDBQueryDuration:
		return "db.query.duration"
	default:
		return "unknown"
	}
}

// ProtocolPrefixRe is the regex, which will be used to remove the protocol prefix from endpoints
const ProtocolPrefixRe = "^\\w+://"

// Telemetry Label
const (
	// LabelHTTPMethod is the label which holds the http method for the http.Handler metric instrumentation
	LabelHTTPMethod string = "http.method"
	// LabelHTTPStatusCode is the label which holds the http response code for the http.Handler metric instrumentation
	LabelHTTPStatusCode string = "http.status_code"
	// LabelGrpcService is the label which holds the grpc service name for the grpc metric instrumentation
	LabelGrpcService string = "rpc.service"
	// LabelDBPoolName is the label which holds the db pool information for the database query execution metrics
	LabelDBPoolName string = "pool.name"
	// LabelPolicyDecision is the label for the policy decision in the metrics
	LabelPolicyDecision string = "decision"
	// LabelPolicyDecisionReason is the label for the decision reason in the metrics
	LabelPolicyDecisionReason string = "reason"
	// LabelRegoPackage is the label for the rego package in the metrics
	LabelRegoPackage string = "rego.package"
)
