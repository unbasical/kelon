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

// entityPaths is just a utility type alias
type entityPaths = map[string]map[string][]string

// callOperands is just a utility type alias
type callOperands = map[string]func(args ...string) (string, error)

type mongoDatastoreTranslator struct {
	appConf     *configs.AppConfig
	alias       string
	entityPaths entityPaths
	callOps     callOperands
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

func (ds *mongoDatastoreTranslator) Execute(_ context.Context, query data.Node) (data.DatastoreQuery, error) {
	if !ds.configured {
		return data.DatastoreQuery{}, errors.Errorf("MongoDatastoreTranslator: Datastore was not configured! Please call Configure().")
	}
	logging.LogForComponent("mongoDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate to map: collection -> filter
	t := newTranslator()
	statement, err := t.Translate(query, ds.entityPaths, ds.callOps)
	if err != nil {
		return data.DatastoreQuery{}, err
	}

	logging.LogForComponent("mongoDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\n", statement)
	return data.DatastoreQuery{Statement: statement}, nil
}

var entityMatcher = regexp.MustCompile(`\{\{(.*?)}}`)

type colFilter struct {
	collection string
	filter     string
}

type translator struct {
	result              map[string]string
	filtersByCollection map[string][]string
	filters             []colFilter
	entities            util.Stack[string]
	relations           util.Stack[string]
	operands            util.Stack[[]string]
	entityPaths         entityPaths
	callOps             callOperands
}

func newTranslator() *translator {
	return &translator{
		result:              make(map[string]string),
		filtersByCollection: make(map[string][]string),
	}
}

func (t *translator) Translate(input data.Node, entityPaths entityPaths, callOps callOperands) (map[string]string, error) {
	t.entityPaths = entityPaths
	t.callOps = callOps

	err := input.Walk(func(node data.Node) error {
		switch n := node.(type) {
		case data.Union:
			return t.walkUnion()
		case data.Query:
			return t.walkQuery()
		case data.Link:
			return t.walkLink()
		case data.Condition:
			return t.walkCondition()
		case data.Conjunction:
			return t.walkConjunction()
		case data.Attribute:
			return t.walkAttribute(n)
		case data.Call:
			return t.walkCall()
		case data.Operator:
			return t.walkOperator(n)
		case data.Entity:
			return t.walkEntity(n)
		case data.Constant:
			return t.walkConstant(n)
		default:

			return errors.Errorf("MongoDatastoreTranslator: Unexpected input: %T -> %+v", n, n)
		}
	})

	return t.result, err
}

func (t *translator) walkUnion() error {
	// Sort collection filters by collection
	for _, colF := range t.filters {
		coll, exists := t.filtersByCollection[colF.collection]
		if exists {
			// Append filter to existing entry
			t.filtersByCollection[colF.collection] = append(coll, colF.filter)
		} else {
			// Write new entry
			t.filtersByCollection[colF.collection] = []string{colF.filter}
		}
	}

	// Combine all filters for each collection with a disjunction
	for collection, filterSlice := range t.filtersByCollection {
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
			if path, found := t.entityPaths[collection][entity]; found {
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

		t.result[collection] = finalFilter
	}
	return nil
}

func (t *translator) walkQuery() error {
	// Expected stack: entities-top -> [singleEntity] relations-top -> [singleCondition]
	var (
		entity    string
		condition string
	)
	// Extract entity
	entity, err := t.entities.Pop()
	if err != nil {
		return err
	}
	// Extract condition
	if !t.relations.IsEmpty() {
		condition, err = t.relations.Peek()
		if err != nil {
			return err
		}
		if t.relations.Size() != 1 {
			return errors.Errorf("MongoDatastoreTranslator: Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", t.relations.Size())
		}
	}

	// Append new filter
	t.filters = append(t.filters, colFilter{
		collection: entity,
		filter:     condition,
	})
	t.relations.Clear()
	return nil
}

// walkLink resets entities because mongo does not join, but can only access directly nested elements!
func (t *translator) walkLink() error {
	t.entities.Clear()
	return nil
}

// walkCondition simply skips the node
func (t *translator) walkCondition() error {
	return nil
}

func (t *translator) walkConjunction() error {
	// Expected stack: relations-top -> [conjunctions ...]
	if !t.relations.IsEmpty() {
		rels := t.relations.Values()
		t.relations.Clear()
		t.relations.Push(fmt.Sprintf("{%s}", strings.Join(rels, ", ")))
		logging.LogForComponent("mongoDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *translator) walkAttribute(a data.Attribute) error {
	// Expected stack:  top -> [entity, ...]
	var entity string
	entity, err := t.entities.Pop()
	if err != nil {
		return err
	}
	// Mark entity with . to be replaced in finished query
	return util.AppendToTop(&t.operands, fmt.Sprintf("\"{{%s.}}%s\"", entity, a.Name))
}

func (t *translator) walkCall() error {
	// Expected stack:  top -> [args..., call-op]
	var ops []string
	ops, err := t.operands.Pop()
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
	if mongoCallOp, ok := t.callOps[op]; ok {
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

	if t.operands.Size() > 0 {
		// If we are in nested call -> push as operand
		if err := util.AppendToTop(&t.operands, nextRel); err != nil {
			return err
		}
	} else {
		// We reached root operation -> relation is processed
		t.relations.Push(nextRel)
		logging.LogForComponent("mongoDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *translator) walkOperator(o data.Operator) error {
	t.operands.Push([]string{})
	return util.AppendToTop(&t.operands, o.String())
}

func (t *translator) walkEntity(e data.Entity) error {
	t.entities.Push(e.String())
	return nil
}

func (t *translator) walkConstant(c data.Constant) error {
	if c.IsNumeric {
		return util.AppendToTop(&t.operands, c.String())
	}

	return util.AppendToTop(&t.operands, fmt.Sprintf("\"%s\"", c.String()))
}
