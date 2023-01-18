package builtins

import (
	"github.com/open-policy-agent/opa/rego"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/topdown/builtins"
	"github.com/open-policy-agent/opa/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var logInfo = &rego.Function{
	Name: "log_info",
	Decl: types.NewVariadicFunction(nil, types.A, nil),
}
var logDebug = &rego.Function{
	Name: "log_debug",
	Decl: types.NewVariadicFunction(nil, types.A, nil),
}
var logWarn = &rego.Function{
	Name: "log_warn",
	Decl: types.NewVariadicFunction(nil, types.A, nil),
}
var logError = &rego.Function{
	Name: "log_error",
	Decl: types.NewVariadicFunction(nil, types.A, nil),
}
var logFatal = &rego.Function{
	Name: "log_fatal",
	Decl: types.NewVariadicFunction(nil, types.A, nil),
}

func makeBuiltinLogFuncForLevel(level log.Level) func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
	return func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
		if !log.IsLevelEnabled(level) {
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

func logWithLevel(logLevel log.Level, operands *ast.Array) error {
	var buf []string

	fillBuf := func(term *ast.Term) error {
		switch v := term.Value.(type) {
		case ast.String:
			buf = append(buf, string(v))
		default:
			buf = append(buf, v.String())
		}
		return nil
	}

	err := operands.Iter(fillBuf)
	if err != nil {
		return err
	}

	switch logLevel {
	case log.InfoLevel:
		log.Info(strings.Join(buf, " "))
	case log.DebugLevel:
		log.Debug(strings.Join(buf, " "))
	case log.WarnLevel:
		log.Warn(strings.Join(buf, " "))
	case log.ErrorLevel:
		log.Error(strings.Join(buf, " "))
	case log.FatalLevel:
		log.Error(strings.Join(buf, " "))
		return topdown.Halt{Err: errors.Errorf("Fatal Log: %s", strings.Join(buf, " "))}
	default:
		return nil
	}

	return nil
}
