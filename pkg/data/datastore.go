// Package data contains components that are used to translate an AST coming from translate.AstTranslator
// into a datastore's native query which can then be executed inside the datastore.
//
// As a result, a final decision for the opa.PolicyCompiler should be the result of query evaluation.
package data

import (
	"context"

	"github.com/unbasical/kelon/configs"
)

// DatastoreTranslator constant types
const (
	TypePostgres = "postgres"
	TypeMysql    = "mysql"
	TypeMongo    = "mongo"
)

// DatastoreQuery holds a prepared query statement and their parameters
type DatastoreQuery struct {
	Statement  any
	Parameters []any
}

// Datastore is the interface that maps a generic designed AST returned by translate.AstTranslator to a native query-statement using the passed data.DatastoreTranslator and execute by the passed data.DatastoreExecutor.
// This should be generally done by translating the Query-AST into the datastore's native query language. If the query returns any result, the datastore should return true, otherwise false.
type Datastore interface {
	// Configure has to be called once before the component can be used (Otherwise Execute will return an error)!
	Configure(appConf *configs.AppConfig, alias string) error

	// Execute translates the given Query-AST into a datastore's native query and executes the query afterward via the passed data.DatastoreExecutor.
	Execute(ctx context.Context, query Node) (bool, error)
}

// DatastoreTranslator is the interface that maps a generic designed AST returned by translate.AstTranslator to a native query-statement which is understood by a matching data.DatastoreExecutor.
// This should be generally done by translating the Query-AST into the datastore's native query language.
type DatastoreTranslator interface {

	// Configure has to be called once before the component can be used (Otherwise Execute will return an error)!
	Configure(appConf *configs.AppConfig, alias string) error

	// Execute translates the given Query-AST into a datastore's native query
	Execute(ctx context.Context, query Node) (DatastoreQuery, error)
}

// DatastoreExecutor is the interface that executes a native datastore query and returns the final decision (Allow/Deny) based on the query response.
// If the query returns any result, the executor returns true, otherwise false.
//
// Please note, that connection-pool-handling should also be done by the DatastoreExecutor internally.
type DatastoreExecutor interface {

	// Configure waits for the attached databases to be reachable by pinging the configured connection every 3 seconds for 1 minute
	// and then configures the Datastore itself.
	//
	// If the database isn't reachable or the Datastore itself fails during this process, the encountered error will be returned (otherwise nil).
	Configure(appConf *configs.AppConfig, alias string) error

	// Execute executes the native query and returns true if the query is not empty and false otherwise.
	Execute(ctx context.Context, query DatastoreQuery) (bool, error)
}

// CallOpMapper is an abstraction for mapping OPA-native functions to DatastoreTranslator-native functions.
// Therefore, each CallOpMapper should provide the OPA-native call operand it handles (i.e. abs) and
// define a function Map() which receives all arguments of the OPA-native function and maps them to a
// datastore's native function call (i.e. ABS(arg)).
//
// Please note, that the mapped function should include the call-operand as well!
type CallOpMapper interface {

	// Handles gets the call-operand this handler handles (i.e. 'abs').
	Handles() string

	// Map returns the corresponding datastore native function (i.e. 'ABS(<arguments>)').
	Map(args ...string) (string, error)
}
