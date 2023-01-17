package data

import (
	"context"
	"fmt"
	"io"
	"strings"

	authzedpb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// checkPermissionCacheKeyType is used as a custom cache key to avoid collisions with other builtins caching data
type checkPermissionCacheKeyType string

// lookupResourceCacheKeyType is used as a custom cache key to avoid collisions with other builtins caching data
type lookupResourceCacheKeyType string

type authzedDatastore struct {
	alias  string
	client *authzed.Client
	conf   *configs.SpiceDB
}

func NewAuthzedDatastore() data.Datastore {
	return &authzedDatastore{}
}

func (s *authzedDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	spiceConf, err := extractAndValidateSpiceDBDatastore(appConf, alias)
	if err != nil {
		return errors.Wrapf(err, "sg")
	}

	s.conf = spiceConf
	s.alias = alias

	var dialOps []grpc.DialOption
	if s.conf.Insecure {
		dialOps = append(dialOps, grpc.WithTransportCredentials(insecure.NewCredentials()), grpcutil.WithInsecureBearerToken(s.conf.Token))
	} else {
		dialOps = append(dialOps, grpcutil.WithSystemCerts(grpcutil.VerifyCA))
		grpcutil.WithBearerToken(s.conf.Token)
	}

	client, err := authzed.NewClient(
		s.conf.Endpoint,
		dialOps...,
	)
	s.client = client

	// Register Builtin
	s.registerBuiltins(alias)

	logging.LogForComponent("authzedDatastore").Infof("Configured")

	return err
}

func (s *authzedDatastore) Execute(ctx context.Context, query data.Node) (bool, error) {
	return false, nil
}

// checkPermission checks the given permission request
//
//nolint:gocritic
func (s *authzedDatastore) checkPermission(bctx rego.BuiltinContext, subjectTerm, permissionTerm, resourceIDTerm *ast.Term) (*ast.Term, error) {
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

	subjectType, subjectID, subjectFound := strings.Cut(subject, ":")
	if !subjectFound {
		return nil, errors.New("could not parse authzdb subject")
	}

	subjectReference := &authzedpb.SubjectReference{Object: &authzedpb.ObjectReference{
		ObjectType: subjectType,
		ObjectId:   subjectID,
	}}

	resourceType, resourceID, resourceFound := strings.Cut(resource, ":")
	resourceReference := &authzedpb.ObjectReference{
		ObjectType: resourceType,
		ObjectId:   resourceID,
	}

	if !resourceFound {
		return nil, errors.New("could not parse authzdb resource")
	}

	resp, err := s.client.CheckPermission(bctx.Context, &authzedpb.CheckPermissionRequest{
		Resource:   resourceReference,
		Permission: permission,
		Subject:    subjectReference,
	})

	if err != nil {
		return nil, err
	}

	result := ast.Boolean(resp.Permissionship == authzedpb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION)
	bctx.Cache.Put(cacheKey, result)

	return ast.NewTerm(result), nil
}

// lookupResources returns all resource ids for a subject, permission and resource type
//
//nolint:gocritic
func (s *authzedDatastore) lookupResources(bctx rego.BuiltinContext, subjectTerm, permissionTerm, resourceTypeTerm *ast.Term) (*ast.Term, error) {
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

	subjectType, subjectID, subjectFound := strings.Cut(subject, ":")
	if !subjectFound {
		return nil, errors.New("could not parse authzdb subject")
	}

	subjectReference := &authzedpb.SubjectReference{Object: &authzedpb.ObjectReference{
		ObjectType: subjectType,
		ObjectId:   subjectID,
	}}

	stream, err := s.client.LookupResources(bctx.Context, &authzedpb.LookupResourcesRequest{
		ResourceObjectType: resourceType,
		Permission:         permission,
		Subject:            subjectReference,
	})

	if err != nil {
		return nil, err
	}

	var resourceIds []string
	result, err := stream.Recv()
	for err == nil {
		resourceIds = append(resourceIds, result.ResourceObjectId)
		result, err = stream.Recv()
	}
	// check for other error then stream finished
	if err != io.EOF {
		return nil, err
	}

	bctx.Cache.Put(cacheKey, resourceIds)
	return ast.ArrayTerm(stringsToTerms(resourceIds...)...), nil
}

func (s *authzedDatastore) registerBuiltins(alias string) {
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
