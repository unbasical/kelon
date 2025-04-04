package constants

type ContextKey string

// ContextKeyRequestID is the ContextKey for RequestID
const ContextKeyRequestID = ContextKey("requestUID") // can be unexported
const ContextKeyRegoPackage = ContextKey("regoPackage")

const Input = "input"

const EndpointData = "/data"
const EndpointPolicies = "/policies"
const EndpointHealth = "/health"
const EndpointMetrics = "/metrics"

const URLParamID = "id"
