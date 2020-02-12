package constants

// MetaKey is used for metadata values nested inside a datastore config. The value requires a key that is not primitive type.
type MetaKey string

// MetaKey for maxOpenConnections
const MetaMaxOpenConnections MetaKey = "maxOpenConnections"
const MetaMaxIdleConnections MetaKey = "maxIdleConnections"
const MetaConnectionMaxLifetimeSeconds MetaKey = "connectionMaxLifetimeSeconds"
const MetaTelemetryName MetaKey = "telemetryName"
const MetaTelemetryType MetaKey = "telemetryType"
