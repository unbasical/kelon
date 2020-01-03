// Package api contains components that handle incoming requests and delegate them to the opa.PolicyCompiler.
package api

import (
	"time"

	"github.com/Foundato/kelon/pkg/opa"

	"github.com/Foundato/kelon/configs"
)

// ClientProxyConfig contains all configuration needed by a single api.ClientProxy to run.
//
// Note that this configuration also contains all configurations for nested components. With this in mind, an
// instance of a ClientProxy can be seen as a standalone thread with all its subcomponents attached to it.
// As a result of that, two ClientProxies that proxy different types of requests (i.e. gRPC and HTTP) should be able
// to run in parallel.
type ClientProxyConfig struct {
	Compiler              *opa.PolicyCompiler
	RespondWithStatusCode bool
	opa.PolicyCompilerConfig
}

// ClientProxy is the interface that serves as the external interface of kelon.
//
// It can implement any external communication standard and should delegate client requests to the opa.PolicyCompiler.
// Note that this component also has access to all sub-components. With this in mind, an
// instance of a ClientProxy can be seen as a standalone thread with all its subcomponents attached to it.
// As a result of that, two ClientProxies that proxy different types of requests (i.e. gRPC and HTTP) should be able
// to run in parallel.
type ClientProxy interface {

	// Configure() first triggers the Configure method of all sub-components and afterwards configures the ClientProxy itself.
	// Please note that Configure has to be called once before the component can be used (Otherwise Start() will return an error)!
	//
	// If any sub-component or the ClientProxy itself fails during this process, the encountered error will be returned (otherwise nil).
	Configure(appConf *configs.AppConfig, serverConf *ClientProxyConfig) error

	// Start() will make the previous configured ClientProxy handle incoming requests.
	//
	// This process should be implemented in a non-blocking manner!
	// If the ClientProxy was not configured before, or any error occurred during startup, an error will be returned (otherwise nil).
	Start() error

	// Stop() will make the ClientProxy to shutdown gracefully.
	//
	// This process should be implemented in a non-blocking manner!
	// If the ClientProxy was not started before, or any error occurred during shutdown, an error will be returned (otherwise nil).
	Stop(deadline time.Duration) error
}
