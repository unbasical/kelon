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
	callOps    map[string]func(args ...string) (string, error)
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

func (ds *sqlDatastoreTranslator) Execute(ctx context.Context, query data.Node) (data.DatastoreQuery, error) {
	if !ds.configured {
		return data.DatastoreQuery{}, errors.Errorf("SqlDatastoreTranslator: DatastoreTranslator was not configured! Please call Configure(). ")
	}
	logging.LogForComponent("sqlDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate query to into sql statement
	statement, params, err := ds.translatePrepared(query)
	if err != nil {
		return data.DatastoreQuery{}, err
	}

	logging.LogForComponent("sqlDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\nPARAMS: %+v", statement, params)

	return data.DatastoreQuery{Statement: statement, Parameters: params}, nil
}

// nolint:gocyclo,gocritic
func (ds *sqlDatastoreTranslator) translatePrepared(input data.Node) (q string, params []interface{}, err error) {
	var query util.Stack[string]
	var selects util.Stack[string]
	var entities util.Stack[string]
	var relations util.Stack[string]
	var joins util.Stack[string]

	var operands util.Stack[[]string]

	// Used for prepared statements
	var values []interface{}

	// Walk input
	err = input.Walk(func(q data.Node) error {
		switch v := q.(type) {
		case data.Union:
			// Expected stack:  top -> [Queries...]
			query.Push(strings.Join(selects.Values(), " UNION "))
			selects.Clear()
		case data.Query:
			// Expected stack: entities-top -> [singleEntity] relations-top -> [singleCondition]
			var (
				entity     string
				joinClause string
				condition  string
			)
			// Extract entity
			entity, err = entities.Pop()
			if err != nil {
				return err
			}
			// Extract joins
			for _, j := range joins.Values() {
				joinClause += j
			}
			// Extract condition
			if !relations.IsEmpty() {
				condition, err = relations.Peek()
				if err != nil {
					return err
				}
				if relations.Size() != 1 {
					return errors.Errorf("SqlDatastoreTranslator: Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", relations.Size())
				}
			}

			//nolint:gosec
			selects.Push(fmt.Sprintf("SELECT count(*) FROM %s%s%s", entity, joinClause, condition))
			joins.Clear()
			relations.Clear()
		case data.Link:
			// Expected stack: entities-top -> [entities]
			for _, entity := range entities.Values() {
				joins.Push(fmt.Sprintf(", %s", entity))
			}
			entities.Clear()
		case data.Condition:
			// Expected stack: relations-top -> [singleRelation]
			if !relations.IsEmpty() {
				var rel string
				rel, err = relations.Pop()
				if err != nil {
					return err
				}
				//nolint:gosec
				relations.Push(fmt.Sprintf(" WHERE %s", rel))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONDITION: relations |%+v <- TOP", relations)
			}
		case data.Disjunction:
			// Expected stack: relations-top -> [disjunctions ...]
			if !relations.IsEmpty() {
				relations.Clear()
				relations.Push(fmt.Sprintf("(%s)", strings.Join(query.Values(), " OR ")))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("DISJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if !relations.IsEmpty() {
				rels := relations.Values()
				relations.Clear()
				relations.Push(fmt.Sprintf("(%s)", strings.Join(rels, " AND ")))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entity, err = entities.Pop()
			if err != nil {
				return err
			}
			if err = util.AppendToTop(&operands, fmt.Sprintf("%s.%s", entity, v.Name)); err != nil {
				return err
			}
		case data.Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			ops, err = operands.Pop()
			if err != nil {
				return err
			}
			op := ops[0]

			// Handle Call
			var nextRel string
			if sqlCallOp, ok := ds.callOps[op]; ok {
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
			if !operands.IsEmpty() {
				// If we are in nested call -> push as operand
				if err = util.AppendToTop(&operands, nextRel); err != nil {
					return err
				}
			} else {
				// We reached root operation -> relation is processed
				relations.Push(nextRel)
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", relations)
			}
		case data.Operator:
			operands.Push([]string{})
			if err = util.AppendToTop(&operands, v.String()); err != nil {
				return err
			}
		case data.Entity:
			schema, entity, schemaError := ds.findSchemaForEntity(v.String())
			if schemaError != nil {
				return schemaError
			}
			if schema == "public" && ds.appConf.Datastores[ds.alias].Type == "postgres" {
				// Special handle when datastore is postgres and schema is public
				entities.Push(entity.Name)
			} else {
				// Normal case for all entities
				entities.Push(fmt.Sprintf("%s.%s", schema, entity.Name))
			}
		case data.Constant:
			values = append(values, v.String())
			if err = util.AppendToTop(&operands, getPreparePlaceholderForPlatform(ds.platform, len(values))); err != nil {
				return err
			}

		default:
			return errors.Errorf("SqlDatastoreTranslator: Unexpected input: %T -> %+v", v, v)
		}
		return nil
	})
	if err != nil {
		logging.LogForComponent("sqlDatastoreTranslator: ").Debug(err)
	}
	return strings.Join(query.Values(), ""), values, err
}

func (ds *sqlDatastoreTranslator) findSchemaForEntity(search string) (string, *configs.Entity, error) {
	// Find custom mapping
	for schema, es := range ds.schemas {
		if found, entity := es.ContainsEntity(search); found {
			return schema, entity, nil
		}
	}
	err := errors.Errorf("SqlDatastoreTranslator: No schema found for entity %s in datastore with alias %s", search, ds.alias)
	return "", &configs.Entity{}, err
}
