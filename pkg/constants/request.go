package constants

type ContextKey string

// ContextKeyRequestID is the ContextKey for RequestID
const ContextKeyRequestID = ContextKey("requestUID") // can be unexported
const ContextKeyRegoPackage = ContextKey("regoPackage")

const HeaderXForwardedMethod = "X-Forwarded-Method"
const HeaderXForwardedURI = "X-Forwarded-URI"
const HeaderAuthorization = "Authorization"

const EndpointSuffixData = "/data"
const EndpointSuffixForwardAuth = "/forward-auth"
const EndpointSuffixPolicies = "/policies"

const EndpointHealth = "/health"
const EndpointMetrics = "/metrics"
