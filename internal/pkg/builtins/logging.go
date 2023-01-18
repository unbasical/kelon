package builtins

import (
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/topdown/builtins"
	"github.com/open-policy-agent/opa/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var logInfo = &ast.Builtin{
	Name:        "log_info",
	Description: "Logs the passed arguments at a info level.",
	Decl:        types.NewVariadicFunction(nil, types.A, nil),
	Categories:  []string{"logging"},
}
var logDebug = &ast.Builtin{
	Name:        "log_debug",
	Description: "Logs the passed arguments at a debug level.",
	Decl:        types.NewVariadicFunction(nil, types.A, nil),
	Categories:  []string{"logging"},
}
var logWarn = &ast.Builtin{
	Name:        "log_warn",
	Description: "Logs the passed arguments at a warn level.",
	Decl:        types.NewVariadicFunction(nil, types.A, nil),
	Categories:  []string{"logging"},
}
var logError = &ast.Builtin{
	Name:        "log_error",
	Description: "Logs the passed arguments at a error level.",
	Decl:        types.NewVariadicFunction(nil, types.A, nil),
	Categories:  []string{"logging"},
}
var logFatal = &ast.Builtin{
	Name:        "log_fatal",
	Description: "Logs the passed arguments at a fatal level.",
	Decl:        types.NewVariadicFunction(nil, types.A, nil),
	Categories:  []string{"logging"},
}

func makeBuiltinLogFuncForLevel(level log.Level) func(topdown.BuiltinContext, []*ast.Term, func(*ast.Term) error) error {
	return func(bctx topdown.BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
		if !log.IsLevelEnabled(level) {
			return iter(nil)
		}

		arr, err := builtins.ArrayOperand(operands[0].Value, 1)
		if err != nil {
			return err
		}

		buf := make([]string, arr.Len())
		err = builtinPrintCrossProductOperands(bctx, buf, level, arr, 0)
		if err != nil {
			return err
		}

		return iter(nil)
	}
}

func builtinPrintCrossProductOperands(bctx topdown.BuiltinContext, buf []string, logLevel log.Level, operands *ast.Array, i int) error {
	if i >= operands.Len() {
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
	}

	xs, ok := operands.Elem(i).Value.(ast.Set)
	if !ok {
		return topdown.Halt{Err: errors.Errorf("illegal argument type: %v", ast.TypeName(operands.Elem(i).Value))}
	}

	if xs.Len() == 0 {
		buf[i] = "<undefined>"
		return builtinPrintCrossProductOperands(bctx, buf, logLevel, operands, i+1)
	}

	return xs.Iter(func(x *ast.Term) error {
		switch v := x.Value.(type) {
		case ast.String:
			buf[i] = string(v)
		default:
			buf[i] = v.String()
		}
		return builtinPrintCrossProductOperands(bctx, buf, logLevel, operands, i+1)
	})
}
