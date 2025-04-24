package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

type sqlDatastoreTranslator struct {
	appConf    *configs.AppConfig
	alias      string
	platform   string
	conn       map[string]string
	schemas    map[string]*configs.EntitySchema
	callOps    callOperands
	configured bool
}

// NewSQLDatastoreTranslator returns a new data.DatastoreTranslator which is able to connect to PostgreSQL and MySQL databases.
func NewSQLDatastoreTranslator() data.DatastoreTranslator {
	return &sqlDatastoreTranslator{
		appConf:    nil,
		alias:      "",
		callOps:    nil,
		configured: false,
	}
}

func (ds *sqlDatastoreTranslator) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Validate config
	conf, e := extractAndValidateDatastore(appConf, alias)
	if e != nil {
		return errors.Wrap(e, "SqlDatastoreTranslator:")
	}
	if schemas, ok := appConf.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("SqlDatastoreTranslator: DatastoreTranslator with alias [%s] has no schemas configured!", alias)
		}

		for schemaName, schema := range schemas {
			if schema.HasNestedEntities() {
				return errors.Errorf("SqlDatastoreTranslator: Schema %q in datastore with alias [%s] contains nested entities which is not supported by SQL-Datastores yet!", schemaName, alias)
			}
		}
	} else {
		return errors.Errorf("SqlDatastoreTranslator: DatastoreTranslator with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Load call handlers
	operands, ok := appConf.CallOperands[conf.Type]
	if !ok {
		return errors.Errorf("no call-operands found for datastore with type [%s]", conf.Type)
	}
	ds.callOps = operands
	logging.LogForComponent("sqlDatastoreTranslator").Infof("SqlDatastoreTranslator [%s] laoded call operands", alias)

	// Assign values
	ds.conn = conf.Connection
	ds.platform = conf.Type
	ds.schemas = appConf.DatastoreSchemas[alias]
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	logging.LogForComponent("sqlDatastoreTranslator").Infof("Configured SqlDatastoreTranslator [%s]", alias)
	return nil
}

func (ds *sqlDatastoreTranslator) Execute(_ context.Context, query data.Node) (data.DatastoreQuery, error) {
	if !ds.configured {
		return data.DatastoreQuery{}, errors.Errorf("SqlDatastoreTranslator: DatastoreTranslator was not configured! Please call Configure(). ")
	}
	logging.LogForComponent("sqlDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate query to into sql statement
	t := newSqlTranslator()
	statement, params, err := t.Translate(query, ds.platform, ds.callOps, ds.schemas)
	if err != nil {
		return data.DatastoreQuery{}, errors.Wrapf(err, "SqlDatastoreTranslator: Translate failed for datastore with alias %s - query: %s", ds.alias, query)
	}

	logging.LogForComponent("sqlDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\nPARAMS: %+v", statement, params)
	return data.DatastoreQuery{Statement: statement, Parameters: params}, nil
}

type sqlTranslator struct {
	platform  string
	callOps   callOperands
	schemas   map[string]*configs.EntitySchema
	query     util.Stack[string]
	selects   util.Stack[string]
	entities  util.Stack[string]
	relations util.Stack[string]
	joins     util.Stack[string]
	operands  util.Stack[[]string]
	values    []any
}

func newSqlTranslator() *sqlTranslator {
	return &sqlTranslator{}
}

func (t *sqlTranslator) Translate(input data.Node, platform string, callOps callOperands, schemas map[string]*configs.EntitySchema) (string, []any, error) {
	t.platform = platform
	t.callOps = callOps
	t.schemas = schemas

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
		case data.Disjunction:
			return t.walkDisjunction()
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
			return errors.Errorf("Unexpected input: %T -> %+v", n, n)
		}
	})

	return strings.Join(t.query.Values(), ""), t.values, err
}

func (t *sqlTranslator) walkUnion() error {
	// Expected stack:  top -> [Queries...]
	t.query.Push(strings.Join(t.selects.Values(), " UNION "))
	t.selects.Clear()
	return nil
}

func (t *sqlTranslator) walkQuery() error {
	// Expected stack: entities-top -> [singleEntity] relations-top -> [singleCondition]
	var (
		entity     string
		joinClause string
		condition  string
	)
	// Extract entity
	entity, err := t.entities.Pop()
	if err != nil {
		return err
	}
	// Extract joins
	for _, j := range t.joins.Values() {
		joinClause += j
	}
	// Extract condition
	if !t.relations.IsEmpty() {
		condition, err = t.relations.Peek()
		if err != nil {
			return err
		}
		if t.relations.Size() != 1 {
			return errors.Errorf("SqlDatastoreTranslator: Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", t.relations.Size())
		}
	}

	//nolint:gosec
	t.selects.Push(fmt.Sprintf("SELECT count(*) FROM %s%s%s", entity, joinClause, condition))
	t.joins.Clear()
	t.relations.Clear()
	return nil
}

func (t *sqlTranslator) walkLink() error {
	// Expected stack: entities-top -> [entities]
	for _, entity := range t.entities.Values() {
		t.joins.Push(fmt.Sprintf(", %s", entity))
	}
	t.entities.Clear()
	return nil
}

func (t *sqlTranslator) walkCondition() error {
	// Expected stack: relations-top -> [singleRelation]
	if !t.relations.IsEmpty() {
		rel, err := t.relations.Pop()
		if err != nil {
			return err
		}
		//nolint:gosec
		t.relations.Push(fmt.Sprintf(" WHERE %s", rel))
		logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONDITION: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *sqlTranslator) walkConjunction() error {
	// Expected stack: relations-top -> [conjunctions ...]
	if !t.relations.IsEmpty() {
		rels := t.relations.Values()
		t.relations.Clear()
		t.relations.Push(fmt.Sprintf("(%s)", strings.Join(rels, " AND ")))
		logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *sqlTranslator) walkDisjunction() error {
	// Expected stack: relations-top -> [disjunctions ...]
	if !t.relations.IsEmpty() {
		t.relations.Clear()
		t.relations.Push(fmt.Sprintf("(%s)", strings.Join(t.query.Values(), " OR ")))
		logging.LogForComponent("sqlDatastoreTranslator").Debugf("DISJUNCTION: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *sqlTranslator) walkAttribute(a data.Attribute) error {
	// Expected stack:  top -> [entity, ...]
	entity, err := t.entities.Pop()
	if err != nil {
		return err
	}
	return util.AppendToTop(&t.operands, fmt.Sprintf("%s.%s", entity, a.Name))
}

func (t *sqlTranslator) walkCall() error {
	// Expected stack:  top -> [args..., call-op]
	ops, err := t.operands.Pop()
	if err != nil {
		return err
	}
	op := ops[0]

	// Handle Call
	var nextRel string
	if sqlCallOp, ok := t.callOps[op]; ok {
		// Expected stack:  top -> [args..., call-op]
		logging.LogForComponent("sqlDatastoreTranslator").Debugln("NEW FUNCTION CALL")
		var callOpError error
		nextRel, callOpError = sqlCallOp(ops[1:]...)
		// Check for call operand error
		if callOpError != nil {
			return callOpError
		}
	} else {
		return errors.Errorf("SqlDatastoreTranslator: Unable to find mapping for operator [%s] in your policy by any of your datastore config!", op)
	}
	if !t.operands.IsEmpty() {
		// If we are in nested call -> push as operand
		if err = util.AppendToTop(&t.operands, nextRel); err != nil {
			return err
		}
	} else {
		// We reached root operation -> relation is processed
		t.relations.Push(nextRel)
		logging.LogForComponent("sqlDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", t.relations)
	}
	return nil
}

func (t *sqlTranslator) walkOperator(o data.Operator) error {
	t.operands.Push([]string{})
	return util.AppendToTop(&t.operands, o.String())
}

func (t *sqlTranslator) walkEntity(e data.Entity) error {
	schema, entity, schemaError := t.findSchemaForEntity(e.String())
	if schemaError != nil {
		return schemaError
	}
	if schema == "public" && t.platform == "postgres" {
		// Special handle when datastore is postgres and schema is public
		t.entities.Push(entity.Name)
	} else {
		// Normal case for all entities
		t.entities.Push(fmt.Sprintf("%s.%s", schema, entity.Name))
	}
	return nil
}

func (t *sqlTranslator) walkConstant(c data.Constant) error {
	t.values = append(t.values, c.String())
	return util.AppendToTop(&t.operands, getPreparePlaceholderForPlatform(t.platform, len(t.values)))
}

func (t *sqlTranslator) findSchemaForEntity(search string) (string, *configs.Entity, error) {
	// Find custom mapping
	for schema, es := range t.schemas {
		if found, entity := es.ContainsEntity(search); found {
			return schema, entity, nil
		}
	}
	err := errors.Errorf("no schema found for entity %s", search)
	return "", &configs.Entity{}, err
}
