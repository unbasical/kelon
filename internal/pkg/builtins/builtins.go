package builtins

import (
	"github.com/open-policy-agent/opa/v1/rego"
	log "github.com/sirupsen/logrus"
)

// RegisterLoggingFunctions registers logging function as buildins for Rego
func RegisterLoggingFunctions() {
	rego.RegisterBuiltinDyn(logInfo, makeBuiltinLogFuncForLevel(log.InfoLevel))
	rego.RegisterBuiltinDyn(logDebug, makeBuiltinLogFuncForLevel(log.DebugLevel))
	rego.RegisterBuiltinDyn(logWarn, makeBuiltinLogFuncForLevel(log.WarnLevel))
	rego.RegisterBuiltinDyn(logError, makeBuiltinLogFuncForLevel(log.ErrorLevel))
	rego.RegisterBuiltinDyn(logFatal, makeBuiltinLogFuncForLevel(log.FatalLevel))
}

// RegisterDatastoreFunction registers datastore specific functions as buildins.
// These functions are configured as call-operands
func RegisterDatastoreFunction(name string, argc int) {
	rego.RegisterBuiltinDyn(makeBuiltinDatastoreFuncDecl(name, argc), makeBuiltinDatastoreFuncImpl())
}
