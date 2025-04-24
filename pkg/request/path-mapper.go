// Package request contains components that help to transform an incoming request into OPA-compatible
// units like a package and a query.
package request

import (
	"fmt"

	"github.com/unbasical/kelon/configs"
)

// PathMapper is the interface that maps an incoming path to a Datastore and a Package.
// This should be enough for the opa.PolicyCompiler to fire a query for partial evaluation
// with the datastore as unknowns.
type PathMapper interface {

	// Configure the PathMapper and returns nil or any encountered error during processors configuration.
	// Please note that Configure has to be called once before the component can be used (Otherwise Map() will return an error)!
	Configure(appConf *configs.AppConfig) error

	// Map an incoming path to a Datastore and a Package.
	//
	// To make the implementation more flexible, the PathMapper itself decides which type of input it needs.
	// Therefore, an appropriate interface like request.pathMapperInput should be used to transport the needed information
	// for path mapping.
	Map(any) (*MapperOutput, error)
}

// PathAmbiguousError is the Error thrown if there are more than one path mapping in the api.yaml-config that match the incoming path.
type PathAmbiguousError struct {
	RequestURL string
	FirstMatch string
	OtherMatch string
}

// PathNotFoundError is the Error thrown if there is no mapping in the api.yaml-config matching the incoming path.
type PathNotFoundError struct {
	RequestURL string
}

// MapperOutput returned by the RequestMapper.
type MapperOutput struct {
	Datastores     []string
	Package        string
	Authorization  bool
	Authentication bool
}

// Textual representation of a PathAmbiguousError.
func (e PathAmbiguousError) Error() string {
	return fmt.Sprintf("Path-mapping [%s] is ambiguous! Mapping [%s] also matches incoming path [%s]!", e.RequestURL, e.FirstMatch, e.OtherMatch)
}

// Textual representation of a PatNotFoundError.
func (e PathNotFoundError) Error() string {
	return fmt.Sprintf("PathMapper: There is no mapping which matches path [%s]!", e.RequestURL)
}
