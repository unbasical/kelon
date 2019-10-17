// Package data contains components that are used to translate an AST coming from translate.AstTranslator
// into a datastore's native query which can then be executed inside the datastore.
//
// As a result, a final decision for the opa.PolicyCompiler should be the result of query evaluation.
package data

import (
	"github.com/Foundato/kelon/configs"
)

// Datastore is the interface that maps a generic designed AST returned by translate.AstTranslator to a final decision (Allow/Deny).
// This should be generally be done by first translating the Query-AST into the datasore's native query language and then
// executing the query. If the query returns any result, the datastore should return true, otherwise false.
//
// Please note, that connection-pool-handling should also be done by the Datastore internally.
type Datastore interface {

	// Configure() waits for the attached databases to be reachable by pinging the configured connection every 3 seconds for 1 minute
	// and then configures the Datastore itself.
	// Please note that Configure has to be called once before the component can be used (Otherwise Execute() will return an error)!
	//
	// If the database isn't reachable or the Datastore itself fails during this process, the encountered error will be returned (otherwise nil).
	Configure(appConf *configs.AppConfig, alias string) error

	// Execute() translates the given Query-AST into a datastore's native query and executes the query afterwards.
	// If the result is not empty, true should be returned otherwise false.
	Execute(query *Node) (bool, error)
}

// CallOpMapper is an abstraction for mapping OPA-native functions to Datastore-native functions.
// Therefore each CallOpMapper should provide the OPA-native call operand it handles (i.e. abs) and
// define a function Map() which receives all arguments of the OPA-native function and maps them to a
// datastore's native function call (i.e. ABS(arg)).
//
// Please note, that the mapped function should include the call-operand as well!
type CallOpMapper interface {

	// Get the call-operand this handler handles (i.e. 'abs').
	Handles() string

	// Return the corresponding datastore native function (i.e. 'ABS(<arguments>)').
	Map(args ...string) string
}
