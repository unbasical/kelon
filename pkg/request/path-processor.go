package request

import (
	"github.com/unbasical/kelon/configs"
)

// PathProcessorConfig contains all configuration needed by a single request.PathProcessor to run.
//
// Note that this configuration also contains all configurations for nested components. With this in mind, an
// instance of a PathProcessor can be seen as a standalone thread with all its subcomponents attached to it.
// As a result of that, two PathProcessors that process different types of paths should be able to run in parallel.
type PathProcessorConfig struct {
	PathMapper *PathMapper
}

// PathProcessorOutput is the result of a successfully mapped path.
//
// Datastore and package should be used to map the incoming request to unknowns and a target package for OPA's partial evaluation.
// Extracted Query-Parameters mapped to their values can i.e. be attached to the input-field of the OPA-query.
// A slice containing all separated path parts is also returned.
type PathProcessorOutput struct {
	Datastores     []string
	Package        string
	Authorization  bool
	Authentication bool
	Path           []string
	Queries        map[string]any
}

// PathProcessor is the interface that processes an incoming path by parsing and afterward mapping it to a Datastore and a Package.
//
// This should be enough for the opa.PolicyCompiler to fire a query for partial evaluation with the datastore as unknowns.
// To separate concerns, the PathProcessor should focus on path parsing and leave the mapping to the request.PathMapper.
type PathProcessor interface {

	// Configure configures the PathProcessor and returns nil or any encountered error during processors configuration.
	// Please note that Configure has to be called once before the component can be used (Otherwise Process() will return an error)!
	Configure(appConf *configs.AppConfig, processorConf *PathProcessorConfig) error

	// Process processes an incoming path by parsing and afterward mapping it to a Datastore and a Package.
	//
	// To make the implementation more flexible, the PathMapper itself decides which type of input it needs.
	// Therefore, an appropriate interface like request.UrlProcessorInput should be used to transport the needed information
	// for path processing.
	Process(input any) (*PathProcessorOutput, error)
}
