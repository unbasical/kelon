package builtins

import (
	"github.com/open-policy-agent/opa/rego"
	log "github.com/sirupsen/logrus"
)

func InitBuiltinFunctions() {
	rego.RegisterBuiltinDyn(logInfo, makeBuiltinLogFuncForLevel(log.InfoLevel))
	rego.RegisterBuiltinDyn(logDebug, makeBuiltinLogFuncForLevel(log.DebugLevel))
	rego.RegisterBuiltinDyn(logWarn, makeBuiltinLogFuncForLevel(log.WarnLevel))
	rego.RegisterBuiltinDyn(logError, makeBuiltinLogFuncForLevel(log.ErrorLevel))
	rego.RegisterBuiltinDyn(logFatal, makeBuiltinLogFuncForLevel(log.FatalLevel))
}
