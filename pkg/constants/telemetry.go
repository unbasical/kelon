package constants

type MetricInstrument int

const (
	InstrumentVersion MetricInstrument = iota
	InstrumentHttpRequestDuration
	InstrumentHttpActiveRequests
	InstrumentHttpRequestSize
	InstrumentRpcRequestDuration
	InstrumentRpcRequestSize
	InstrumentDecisionDuration
	InstrumentDbQueryDuration
)

func (i MetricInstrument) String() string {
	switch i {
	case InstrumentVersion:
		return "version"
	case InstrumentHttpRequestDuration:
		return "http.server.duration"
	case InstrumentHttpActiveRequests:
		return "http.server.active_requests"
	case InstrumentHttpRequestSize:
		return "http.server.request.size"
	case InstrumentRpcRequestDuration:
		return "rpc.server.duration"
	case InstrumentRpcRequestSize:
		return "rpc.server.request.size"
	case InstrumentDecisionDuration:
		return "decision.duration"
	case InstrumentDbQueryDuration:
		return "db.query.duration"
	default:
		return "unknown"
	}
}

const LabelHttpMethod string = "http.method"

const LabelHttpStatusCode string = "http.status_code"

const LabelGrpcService string = "rpc.service"

const LabelDbPoolName string = "pool.name"

const LabelPolicyDecision string = "decision"

const LabelRegoPackage string = "rego.package"
