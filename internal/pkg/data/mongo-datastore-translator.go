package data

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type mongoDatastoreTranslator struct {
	appConf     *configs.AppConfig
	alias       string
	entityPaths map[string]map[string][]string
	callOps     map[string]func(args ...string) (string, error)
	configured  bool
}

type mongoQueryResult struct {
	err   error
	count int64
}

// NewMongoDatastoreTranslator Returns a new data.DatastoreTranslator which is able to connect to MongoDB databases.
func NewMongoDatastoreTranslator() data.DatastoreTranslator {
	return &mongoDatastoreTranslator{
		appConf:    nil,
		alias:      "",
		callOps:    nil,
		configured: false,
	}
}

func (ds *mongoDatastoreTranslator) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Validate config
	conf, err := extractAndValidateDatastore(appConf, alias)
	if err != nil {
		return errors.Wrap(err, "MongoDatastoreTranslator:")
	}
	if schemas, ok := appConf.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("MongoDatastoreTranslator: DatastoreTranslator with alias [%s] has no schemas configured!", alias)
		}
	} else {
		return errors.Errorf("MongoDatastoreTranslator: DatastoreTranslator with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Load call handlers
	operands, ok := appConf.CallOperands[conf.Type]
	if !ok {
		return errors.Errorf("no call-operands found for datastore with type [%s]", conf.Type)
	}
	ds.callOps = operands
	logging.LogForComponent("mongoDatastoreTranslator").Infof("[%s] loaded call operands", alias)

	// Load entity schemas
	ds.entityPaths = make(map[string]map[string][]string)
	for _, schema := range appConf.DatastoreSchemas[alias] {
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

func (ds *mongoDatastoreTranslator) Execute(ctx context.Context, query data.Node) (data.DatastoreQuery, error) {
	if !ds.configured {
		return data.DatastoreQuery{}, errors.Errorf("MongoDatastoreTranslator: Datastore was not configured! Please call Configure().")
	}
	logging.LogForComponent("mongoDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate to map: collection -> filter
	statements, err := ds.translate(query)
	if err != nil {
		return data.DatastoreQuery{}, err
	}

	logging.LogForComponent("mongoDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\n", statements)

	return data.DatastoreQuery{Statement: statements}, nil
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
	var entities util.Stack[string]
	var relations util.Stack[string]
	var operands util.Stack[[]string]
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
					err = errors.Errorf("MongoDatastoreTranslator: Unable to find mapping for entity %q in collection %q", entity, collection)
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
			entity, err := entities.Pop()
			if err != nil {
				return err
			}
			// Extract condition
			if !relations.IsEmpty() {
				condition, err = relations.Peek()
				if err != nil {
					return err
				}
				if relations.Size() != 1 {
					return errors.Errorf("MongoDatastoreTranslator: Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", relations.Size())
				}
			}

			// Append new filter
			filters = append(filters, colFilter{
				collection: entity,
				filter:     condition,
			})
			relations.Clear()
		case data.Link:
			// Reset entities because mongo does not join, but can only access directly nested elements!
			entities.Clear()
		case data.Condition:
			// Skip condition
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if !relations.IsEmpty() {
				rels := relations.Values()
				relations.Clear()
				relations.Push(fmt.Sprintf("{%s}", strings.Join(rels, ", ")))
				logging.LogForComponent("mongoDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entity, err := entities.Pop()
			if err != nil {
				return err
			}
			// Mark entity with . to be replaced in finished query
			if err := util.AppendToTop(&operands, fmt.Sprintf("\"{{%s.}}%s\"", entity, v.Name)); err != nil {
				return err
			}
		case data.Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			ops, err := operands.Pop()
			if err != nil {
				return err
			}
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
				return errors.Errorf("MongoDatastoreTranslator: Unable to find mapping for operator [%s] in your policy by any of your datastore config!", op)
			}

			if operands.Size() > 0 {
				// If we are in nested call -> push as operand
				if err := util.AppendToTop(&operands, nextRel); err != nil {
					return err
				}
			} else {
				// We reached root operation -> relation is processed
				relations.Push(nextRel)
				logging.LogForComponent("mongoDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", relations)
			}
		case data.Operator:
			operands.Push([]string{})
			if err := util.AppendToTop(&operands, v.String()); err != nil {
				return err
			}
		case data.Entity:
			entities.Push(v.String())
		case data.Constant:
			if v.IsNumeric {
				if err := util.AppendToTop(&operands, v.String()); err != nil {
					return err
				}
			} else {
				if err := util.AppendToTop(&operands, fmt.Sprintf("\"%s\"", v.String())); err != nil {
					return err
				}
			}
		default:
			// Stop function in case of error
			return errors.Errorf("MongoDatastoreTranslator: Unexpected input: %T -> %+v", v, v)
		}
		return nil
	})
	if err != nil {
		logging.LogForComponent("mongoDatastoreTranslator").Debug(err)
	}
	return result, err
}
