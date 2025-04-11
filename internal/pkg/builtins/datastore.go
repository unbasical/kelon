package builtins

import (
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
)

// makeBuiltinDatastoreFuncImpl creates a dummy buildin rego function for the call-operand datastore function
func makeBuiltinDatastoreFuncImpl() func(_ rego.BuiltinContext, _ []*ast.Term) (*ast.Term, error) {
	return func(_ rego.BuiltinContext, _ []*ast.Term) (*ast.Term, error) {
		return ast.NullTerm(), nil
	}
}

// makeBuiltinDatastoreFuncDecl creates the rego function definition for the datastore call-operand function
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
