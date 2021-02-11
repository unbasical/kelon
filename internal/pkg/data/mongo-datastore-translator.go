package data

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Foundato/kelon/pkg/constants/logging"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/pkg/errors"
)

type mongoDatastoreTranslator struct {
	appConf     *configs.AppConfig
	alias       string
	entityPaths map[string]map[string][]string
	callOps     map[string]func(args ...string) (string, error)
	configured  bool
	executor    data.DatastoreExecutor
}

type mongoQueryResult struct {
	err   error
	count int64
}

// Return a new data.DatastoreTranslator which is able to connect to PostgreSQL and MySQL databases.
func NewMongoDatastore(executor data.DatastoreExecutor) data.DatastoreTranslator {
	return &mongoDatastoreTranslator{
		appConf:    nil,
		alias:      "",
		callOps:    nil,
		configured: false,
		executor:   executor,
	}
}

func (ds *mongoDatastoreTranslator) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Configure executer
	if ds.executor == nil {
		return errors.Errorf("MongoDatastoreTranslator: DatastoreExecutor not configured!")
	}
	if err := ds.executor.Configure(appConf, alias); err != nil {
		return errors.Wrap(err, "MongoDatastoreTranslator: Error while configuring datastore executor")
	}

	// Validate config
	conf, err := extractAndValidateDatastore(appConf, alias)
	if err != nil {
		return errors.Wrap(err, "MongoDatastoreTranslator:")
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("MongoDatastoreTranslator: DatastoreTranslator with alias [%s] has no schemas configured!", alias)
		}
	} else {
		return errors.Errorf("MongoDatastoreTranslator: DatastoreTranslator with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Load call handlers
	operands, err := loadCallOperands(conf)
	if err != nil {
		return errors.Wrap(err, "MongoDatastoreTranslator:")
	}
	ds.callOps = operands
	logging.LogForComponent("mongoDatastoreTranslator").Infof("[%s] loaded call operands", alias)

	// Load entity schemas
	ds.entityPaths = make(map[string]map[string][]string)
	for _, schema := range appConf.Data.DatastoreSchemas[alias] {
		paths := schema.GenerateEntityPaths()
		for k, v := range paths {
			ds.entityPaths[k] = v
		}
	}

	// Assign values
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	logging.LogForComponent("mongoDatastoreTranslator").Infof("Configured [%s]", alias)
	return nil
}

func (ds *mongoDatastoreTranslator) Execute(query data.Node) (bool, error) {
	if !ds.configured {
		return false, errors.Errorf("MongoDatastore was not configured! Please call Configure().")
	}
	logging.LogForComponent("mongoDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate to map: collection -> filter
	statements, err := ds.translate(query)
	if err != nil {
		return false, err
	}

	logging.LogForComponent("mongoDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\n", statements)

	return ds.executor.Execute(statements, nil)
}

// nolint:gocyclo,gocritic
func (ds *mongoDatastoreTranslator) translate(input data.Node) (map[string]string, error) {
	type colFilter struct {
		collection string
		filter     string
	}
	entityMatcher := regexp.MustCompile(`\{\{(.*?)\}\}`)
	result := make(map[string]string)
	filtersByCollection := make(map[string][]string)
	var filters []colFilter
	var entities util.SStack
	var relations util.SStack
	var operands util.OpStack
	// Walk input
	err := input.Walk(func(q data.Node) error {
		switch v := q.(type) {
		case data.Union:
			// Sort collection filters by collection
			for _, colF := range filters {
				coll, exists := filtersByCollection[colF.collection]
				if exists {
					// Append filter to existing entry
					filtersByCollection[colF.collection] = append(coll, colF.filter)
				} else {
					// Write new entry
					filtersByCollection[colF.collection] = []string{colF.filter}
				}
			}

			// Combine all filters for each collection with a disjunction
			for collection, filterSlice := range filtersByCollection {
				combinedFilter := fmt.Sprintf("{ \"$or\": [ %s ] }", strings.Join(filterSlice, ", "))

				// Postprocessing
				// If the collection is i.e. apps, then remove all occurrences of 'apps.' in the mapped filters
				collection := collection // pin!
				var err error
				finalFilter := entityMatcher.ReplaceAllStringFunc(combinedFilter, func(match string) string {
					entity := match[2 : len(match)-3] // Extract entity. Each match has format: {{<entity>.}}

					// the collection is root level and therefore entirely removed
					if entity == collection {
						return ""
					}

					// All other entities are mapped to final paths
					if path, found := ds.entityPaths[collection][entity]; found {
						// Skip collection in path
						return strings.Join(path[1:], ".") + "."
					}
					err = errors.Errorf("Unable to find mapping for entity %q in collection %q", entity, collection)
					return ""
				})

				// Stop function in case of error
				if err != nil {
					return err
				}

				result[collection] = finalFilter
			}
		case data.Query:
			// Expected stack: entities-top -> [singleEntity] relations-top -> [singleCondition]
			var (
				entity    string
				condition string
			)
			// Extract entity
			entities, entity = entities.Pop()
			// Extract condition
			if len(relations) > 0 {
				condition = relations[0]
				if len(relations) != 1 {
					return errors.Errorf("Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", len(relations))
				}
			}

			// Append new filter
			filters = append(filters, colFilter{
				collection: entity,
				filter:     condition,
			})
			relations = relations[:0]
		case data.Link:
			// Reset entities because mongo does not join, but can only access directly nested elements!
			entities = entities[:0]
		case data.Condition:
			// Skip condition
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("{%s}", strings.Join(relations, ", ")))
				logging.LogForComponent("mongoDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entities, entity = entities.Pop()
			// Mark entity with . to be replaced in finished query
			operands.AppendToTop(fmt.Sprintf("\"{{%s.}}%s\"", entity, v.Name))
		case data.Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			operands, ops = operands.Pop()
			op := ops[0]

			// Sort call operands in case of eq operation
			// This has to be done because MongoDB maps equality to normal JSON-Attributes.
			if len(ops) == 3 {
				// Check if first operand is not an entity
				if !entityMatcher.MatchString(ops[1]) {
					// Swap operands
					ops[1], ops[2] = ops[2], ops[1]
				}
			}

			// Handle Call
			var nextRel string
			if mongoCallOp, ok := ds.callOps[op]; ok {
				// Expected stack:  top -> [args..., call-op]
				logging.LogForComponent("mongoDatastoreTranslator").Debugln("NEW FUNCTION CALL")
				var callOpError error
				nextRel, callOpError = mongoCallOp(ops[1:]...)
				// Check for call operand error
				if callOpError != nil {
					return callOpError
				}
			} else {
				// Stop function in case of error
				return errors.Errorf("Datastores: Operator [%s] is not supported!", op)
			}

			if len(operands) > 0 {
				// If we are in nested call -> push as operand
				operands.AppendToTop(nextRel)
			} else {
				// We reached root operation -> relation is processed
				relations = relations.Push(nextRel)
				logging.LogForComponent("mongoDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", relations)
			}
		case data.Operator:
			operands = operands.Push([]string{})
			operands.AppendToTop(v.String())
		case data.Entity:
			entities = entities.Push(v.String())
		case data.Constant:
			if v.IsNumeric {
				operands.AppendToTop(v.String())
			} else {
				operands.AppendToTop(fmt.Sprintf("\"%s\"", v.String()))
			}
		default:
			// Stop function in case of error
			return errors.Errorf("Unexpected input: %T -> %+v", v, v)
		}
		return nil
	})
	if err != nil {
		logging.LogForComponent("mongoDatastoreTranslator").Debug(err)
	}
	return result, errors.Wrap(err, "mongoDatastoreTranslator")
}
