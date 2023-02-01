package builtins

import (
	"github.com/open-policy-agent/opa/rego"
	log "github.com/sirupsen/logrus"
)

func RegisterLoggingFunctions() {
	rego.RegisterBuiltinDyn(logInfo, makeBuiltinLogFuncForLevel(log.InfoLevel))
	rego.RegisterBuiltinDyn(logDebug, makeBuiltinLogFuncForLevel(log.DebugLevel))
	rego.RegisterBuiltinDyn(logWarn, makeBuiltinLogFuncForLevel(log.WarnLevel))
	rego.RegisterBuiltinDyn(logError, makeBuiltinLogFuncForLevel(log.ErrorLevel))
	rego.RegisterBuiltinDyn(logFatal, makeBuiltinLogFuncForLevel(log.FatalLevel))
}

func RegisterDatastoreFunction(name string, argc int) {
	rego.RegisterBuiltinDyn(makeBuiltinDatastoreFuncDecl(name, argc), makeBuiltinDatastoreFuncImpl())
}
