package builtins

import (
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"
	"github.com/open-policy-agent/opa/v1/topdown/builtins"
	"github.com/open-policy-agent/opa/v1/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/pkg/constants/logging"
)

// nolint:gochecknoglobals,gocritic
var (
	// logInfo is a buildin log function for rego, logging on the INFO level
	logInfo = &rego.Function{
		Name: "log_info",
		Decl: types.NewVariadicFunction(nil, types.A, nil),
	}
	// logDebug is a buildin log function for rego, logging on the DEBUG level
	logDebug = &rego.Function{
		Name: "log_debug",
		Decl: types.NewVariadicFunction(nil, types.A, nil),
	}
	// logWarn is a buildin log function for rego, logging on the WARN level
	logWarn = &rego.Function{
		Name: "log_warn",
		Decl: types.NewVariadicFunction(nil, types.A, nil),
	}
	// logError is a buildin log function for rego, logging on the ERROR level
	logError = &rego.Function{
		Name: "log_error",
		Decl: types.NewVariadicFunction(nil, types.A, nil),
	}
	// logFatal is a buildin log function for rego, logging on the FATAL level
	logFatal = &rego.Function{
		Name: "log_fatal",
		Decl: types.NewVariadicFunction(nil, types.A, nil),
	}
)

// makeBuiltinLogFuncForLevel constructs a OPA buildin function, which logs the provided operands for the provided logrus.Level
func makeBuiltinLogFuncForLevel(level logrus.Level) func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
	return func(_ rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
		if !logrus.IsLevelEnabled(level) {
			return ast.NullTerm(), nil
		}

		arr, err := builtins.ArrayOperand(terms[0].Value, 1)
		if err != nil {
			return nil, err
		}

		err = logWithLevel(level, arr)
		if err != nil {
			return nil, err
		}

		return ast.NullTerm(), nil
	}
}

// logWithLevel tries to log all operands as a single line with the given logrus.Level
func logWithLevel(logLevel logrus.Level, operands *ast.Array) error {
	buf := make([]string, operands.Len())
	idx := 0

	fillBuf := func(term *ast.Term) error {
		switch v := term.Value.(type) {
		case ast.String:
			buf[idx] = string(v)
		default:
			buf[idx] = v.String()
		}
		idx++
		return nil
	}

	err := operands.Iter(fillBuf)
	if err != nil {
		return err
	}

	switch logLevel {
	case logrus.InfoLevel:
		logging.LogForComponent("policy").Info(strings.Join(buf, " "))
	case logrus.DebugLevel:
		logging.LogForComponent("policy").Debug(strings.Join(buf, " "))
	case logrus.WarnLevel:
		logging.LogForComponent("policy").Warn(strings.Join(buf, " "))
	case logrus.ErrorLevel:
		logging.LogForComponent("policy").Error(strings.Join(buf, " "))
	case logrus.FatalLevel:
		logging.LogForComponent("policy").Error(strings.Join(buf, " "))
		return topdown.Halt{Err: errors.Errorf("Fatal Log: %s", strings.Join(buf, " "))}
	default:
		return nil
	}

	return nil
}
