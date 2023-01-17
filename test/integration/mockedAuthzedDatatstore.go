package integration

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

// checkPermissionCacheKeyType is used as a custom cache key to avoid collisions with other builtins caching data
type checkPermissionCacheKeyType string

// lookupResourceCacheKeyType is used as a custom cache key to avoid collisions with other builtins caching data
type lookupResourceCacheKeyType string

type mockAuthzedDatastore struct {
	alias string
}

func NewMockAuthzedDatastore() data.Datastore {
	return &mockAuthzedDatastore{}
}

func (s *mockAuthzedDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	// Register Builtin
	s.registerBuiltins(alias)

	logging.LogForComponent("mockAuthzedDatastore").Infof("Configured")

	return nil
}

func (s *mockAuthzedDatastore) Execute(ctx context.Context, query data.Node) (bool, error) {
	return true, nil
}

// checkPermission checks the given permission request
//
//nolint:gocritic
func (s *mockAuthzedDatastore) checkPermission(bctx rego.BuiltinContext, subjectTerm, permissionTerm, resourceIDTerm *ast.Term) (*ast.Term, error) {
	var resource string
	var permission string
	var subject string

	if err := ast.As(resourceIDTerm.Value, &resource); err != nil {
		return nil, err
	}
	if err := ast.As(permissionTerm.Value, &permission); err != nil {
		return nil, err
	}
	if err := ast.As(subjectTerm.Value, &subject); err != nil {
		return nil, err
	}

	// Check if it is already cached, assume they never become invalid.
	var cacheKey = checkPermissionCacheKeyType(fmt.Sprintf("%s.%s#%s@%s", s.alias, subject, permission, resource))
	cached, ok := bctx.Cache.Get(cacheKey)
	if ok {
		return ast.NewTerm(cached.(ast.Value)), nil
	}

	result := ast.Boolean(true)
	bctx.Cache.Put(cacheKey, result)

	return ast.NewTerm(result), nil
}

// lookupResources returns all resource ids for a subject, permission and resource type
//
//nolint:gocritic
func (s *mockAuthzedDatastore) lookupResources(bctx rego.BuiltinContext, subjectTerm, permissionTerm, resourceTypeTerm *ast.Term) (*ast.Term, error) {
	var resourceType string
	var permission string
	var subject string

	if err := ast.As(resourceTypeTerm.Value, &resourceType); err != nil {
		return nil, err
	}
	if err := ast.As(permissionTerm.Value, &permission); err != nil {
		return nil, err
	}
	if err := ast.As(subjectTerm.Value, &subject); err != nil {
		return nil, err
	}

	// Check if it is already cached, assume they never become invalid.
	var cacheKey = lookupResourceCacheKeyType(fmt.Sprintf("%s.%s#%s@%s", s.alias, subject, permission, resourceType))
	cached, ok := bctx.Cache.Get(cacheKey)
	if ok {
		resourceIds := cached.([]string)
		return ast.ArrayTerm(stringsToTerms(resourceIds...)...), nil
	}

	var resourceIds []string

	bctx.Cache.Put(cacheKey, resourceIds)
	return ast.ArrayTerm(stringsToTerms(resourceIds...)...), nil
}

func (s *mockAuthzedDatastore) registerBuiltins(alias string) {
	permissionCheck := &rego.Function{Name: fmt.Sprintf("%s.permission_check", alias),
		Decl: types.NewFunction(
			types.Args(
				types.S, // subject
				types.S, // permission
				types.S, // resource
			),
			types.B), // Returns a boolean
	}

	lookupResources := &rego.Function{Name: fmt.Sprintf("%s.lookup_resources", alias),
		Decl: types.NewFunction(
			types.Args(
				types.S, // subject
				types.S, // permission
				types.S, // resource type
			),
			types.NewArray(nil, types.S)), // Returns a string array (resource ids)
	}

	rego.RegisterBuiltin3(permissionCheck, s.checkPermission)
	rego.RegisterBuiltin3(lookupResources, s.lookupResources)
}

func stringsToTerms(values ...string) []*ast.Term {
	var output []*ast.Term

	for _, s := range values {
		output = append(output, ast.StringTerm(s))
	}

	return output
}
