package constants

type MetricInstrument int

const (
	InstrumentVersion MetricInstrument = iota
	InstrumentHTTPRequestDuration
	InstrumentHTTPActiveRequests
	InstrumentHTTPRequestSize
	InstrumentRPCRequestDuration
	InstrumentRPCRequestSize
	InstrumentDecisionDuration
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

const ProtocolPrefixRe = "^\\w+://"

const LabelHTTPMethod string = "http.method"

const LabelHTTPStatusCode string = "http.status_code"

const LabelGrpcService string = "rpc.service"

const LabelDBPoolName string = "pool.name"

const LabelPolicyDecision string = "decision"

const LabelPolicyDecisionReason string = "reason"

const LabelRegoPackage string = "rego.package"
