package builtins

import (
	"slices"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/types"
	"github.com/unbasical/kelon/pkg/authn"
	"github.com/unbasical/kelon/pkg/constants/logging"
)

// makeJwtAuthFuncDecl returns function decl jwt_verify(jwt string, auth_configs []string) bool
func makeJwtAuthFuncDecl() *rego.Function {
	arr := types.NewArray([]types.Type{types.S}, types.S)
	jwt := types.S
	args := []types.Type{jwt, arr}

	return &rego.Function{
		Name: "jwt_verify",
		Decl: types.NewFunction(args, types.B),
	}
}

func makeBuiltinJwtAuthFuncImpl(authenticators []authn.Authenticator) rego.Builtin2 {
	return func(bctx topdown.BuiltinContext, a, b *ast.Term) (*ast.Term, error) {
		var (
			jwt     string
			aliases []string
		)

		if err := ast.As(a.Value, &jwt); err != nil {
			return nil, err
		}

		if err := ast.As(b.Value, &aliases); err != nil {
			return nil, err
		}

		for _, a := range authenticators {
			if slices.Contains(aliases, a.Alias()) {
				valid, err := a.Authenticate(bctx.Context, jwt)
				if err != nil {
					logging.LogForComponent("builtin").WithError(err).Warnf("error occurred during JWT validation in authenticator [%s]", a.Alias())
					continue
				}
				if valid {
					return ast.BooleanTerm(true), nil
				}
			}
		}

		return ast.BooleanTerm(false), nil
	}
}
