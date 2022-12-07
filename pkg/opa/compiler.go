// Package opa contains components that generate decisions on incoming requests.
//
// This is done by using OPA's partial evaluation API and then translating the partial evaluated
// result into Datastore native query statements which will be used to make the final decision.
package opa

import (
	"context"

	"github.com/open-policy-agent/opa/plugins"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/request"
	"github.com/unbasical/kelon/pkg/translate"
	"github.com/unbasical/kelon/pkg/watcher"
)

// PolicyCompilerConfig contains all configuration needed by a single opa.PolicyCompiler to run.
//
// Note that this configuration also contains all configurations for nested components. With this in mind, an
// instance of a PolicyCompiler can be seen as a standalone thread with all its subcomponents attached to it.
// As a result, two PolicyCompilers should be able to run in parallel.
type PolicyCompilerConfig struct {
	OpaConfigPath *string
	RegoDir       *string
	Prefix        *string
	PathProcessor *request.PathProcessor
	Translator    *translate.AstTranslator
	ConfigWatcher *watcher.ConfigWatcher
	translate.AstTranslatorConfig
	request.PathProcessorConfig
	AccessDecisionLogLevel string
}

type Decision struct {
	Verify  bool
	Allow   bool
	Package string
	Path    string
	Method  string
}

// PolicyCompiler is the interface that makes final decisions on incoming requests.
//
// Its main task is to parse the incoming requests, compile them using OPA's partial evaluation,
// translate the partial evaluated AST into a datastore's native query (which will be executed) and evaluate the query result.
//
// Note that this component also has access to all sub-components. With this in mind, an
// instance of a PolicyCompiler can be seen as a standalone thread with all its subcomponents attached to it.
// As a result, two PolicyCompilers should be able to run in parallel.
type PolicyCompiler interface {

	// Configure() first triggers the Configure method of all sub-components and afterwards configures the PolicyCompiler itself.
	// Please note that Configure has to be called once before the component can be used (Otherwise Process() will return an error)!
	//
	// If any sub-component or the PolicyCompiler itself fails during this process, the encountered error will be returned (otherwise nil).
	Configure(appConfig *configs.AppConfig, compConfig *PolicyCompilerConfig) error

	// Get the underlying open policy agent which is running inside the PolicyCompiler.
	GetEngine() *plugins.Manager

	Execute(ctx context.Context, request map[string]interface{}) (*Decision, error)
}
