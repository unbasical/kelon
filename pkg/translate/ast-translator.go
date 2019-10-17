// Package translate contains components that help to process a partially evaluated AST returned by OPA.
//
// As a result, a final decision for this AST should be returned (Allow/Deny) as boolean.
// There is the possibility to use data.Datastore internally to evaluate queries inside datastores.
package translate

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/open-policy-agent/opa/rego"
)

// AstTranslatorConfig contains all configuration needed by a single translate.AstTranslator to run.
//
// Note that this configuration also contains all configurations for nested components. With this in mind, an
// instance of a AstTranslator can be seen as a standalone thread with all its subcomponents attached to it.
// As a result, two AstTranslators should be able to run in parallel.
type AstTranslatorConfig struct {
	Datastores map[string]*data.Datastore
}

// AstTranslator is the interface that maps a partially evaluated AST returned by OPA to a final decision (Allow/Deny).
// This can either be done by just processing the AST or by using a Datastore to evaluate the translated query inside an external datasource.
//
// To use a datastore, the AstTranslator has to translate the partial evaluated AST from OPA into an intermediate format which is basically another AST with a
// root node of type data.node. This intermediate ast should be then translated into a datastore-native query by the datastore itself.
type AstTranslator interface {

	// Configure() first triggers the Configure method of all sub-components and afterwards configures the AstTranslator itself.
	// Please note that Configure has to be called once before the component can be used (Otherwise Process() will return an error)!
	//
	// If any sub-component or the AstTranslator itself fails during this process, the encountered error will be returned (otherwise nil).
	Configure(appConf *configs.AppConfig, transConf *AstTranslatorConfig) error

	// Process() evaluates a list of partial evaluated OPA-queries by generally translating them to a AST with root-node of type data.Node.
	// This AST is then handed over to a Datastore to be translated into a datastore-native query which will be executed and interpreted as a final decision (Allow/Deny).
	//
	// If any error occurred during the translation or the datastore access, the error will be returned.
	Process(response *rego.PartialQueries, datastore string) (bool, error)
}
