package constants

type ContextKey string

// ContextKeyRequestID is the ContextKey for RequestID
const ContextKeyRequestID = ContextKey("requestUID") // can be unexported
const ContextKeyRegoPackage = ContextKey("regoPackage")
