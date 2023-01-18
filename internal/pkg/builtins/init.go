package builtins

import (
	"strings"

	"github.com/unbasical/kelon/pkg/constants/logging"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/topdown"
	log "github.com/sirupsen/logrus"
)

func InitBuiltinFunctions() {
	ast.Builtins = append(ast.Builtins,
		logInfo,
		logDebug,
		logWarn,
		logError,
		logFatal,
	)
	topdown.RegisterBuiltinFunc("log_info", makeBuiltinLogFuncForLevel(log.InfoLevel))
	topdown.RegisterBuiltinFunc("log_debug", makeBuiltinLogFuncForLevel(log.DebugLevel))
	topdown.RegisterBuiltinFunc("log_warn", makeBuiltinLogFuncForLevel(log.WarnLevel))
	topdown.RegisterBuiltinFunc("log_error", makeBuiltinLogFuncForLevel(log.ErrorLevel))
	topdown.RegisterBuiltinFunc("log_fatal", makeBuiltinLogFuncForLevel(log.FatalLevel))
	logging.LogForComponent("builtins").Info("Loaded OPA builtins")
}

func GetCapabilities() *ast.Capabilities {
	capabilities, err := ast.LoadCapabilitiesJSON(strings.NewReader(`
{
  "builtins": [
    {"name": "log_info","decl": {"type": "function","variadic": {"type": "any"}}},
    {"name": "log_debug","decl": {"type": "function","variadic": {"type": "any"}}},
    {"name": "log_warn","decl": {"type": "function","variadic": {"type": "any"}}},
    {"name": "log_error","decl": {"type": "function","variadic": {"type": "any"}}},
    {"name": "log_fatal","decl": {"type": "function","variadic": {"type": "any"}}},
  ]
}
`))
	if err != nil {
		panic(err)
	}

	return capabilities
}
