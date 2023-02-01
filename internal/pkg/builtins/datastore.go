package builtins

import (
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
)

func makeBuiltinDatastoreFuncImpl() func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
	return func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
		return ast.NullTerm(), nil
	}
}

func makeBuiltinDatastoreFuncDecl(name string, argc int) *rego.Function {
	args := make([]types.Type, argc)
	for i := 0; i < argc; i++ {
		args[i] = types.A
	}

	return &rego.Function{
		Name:             name,
		Decl:             types.NewFunction(args, types.A),
		Nondeterministic: true,
	}
}
