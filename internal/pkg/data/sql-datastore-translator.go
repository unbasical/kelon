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
	executor   data.DatastoreExecutor
}

// Return a new data.DatastoreTranslator which is able to connect to PostgreSQL and MySQL databases.
func NewSQLDatastore(executor data.DatastoreExecutor) data.DatastoreTranslator {
	if executor == nil {
		logging.LogForComponent("sqlDatastoreTranslator").Panic("Nil is not a valid argument for executor")
	}

	return &sqlDatastoreTranslator{
		appConf:    nil,
		alias:      "",
		callOps:    nil,
		configured: false,
		executor:   executor,
	}
}

func (ds *sqlDatastoreTranslator) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Configure executor
	if ds.executor == nil {
		return errors.Errorf("SqlDatastoreTranslator: DatastoreExecutor not configured!")
	}
	if err := ds.executor.Configure(appConf, alias); err != nil {
		return errors.Wrap(err, "SqlDatastoreTranslator: Error while configuring datastore executor")
	}

	// Validate config
	conf, e := extractAndValidateDatastore(appConf, alias)
	if e != nil {
		return errors.Wrap(e, "SqlDatastoreTranslator:")
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
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
	operands, err := loadCallOperands(conf)
	if err != nil {
		return errors.Wrap(err, "SqlDatastoreTranslator:")
	}
	ds.callOps = operands
	logging.LogForComponent("sqlDatastoreTranslator").Infof("SqlDatastoreTranslator [%s] laoded call operands", alias)

	// Assign values
	ds.conn = conf.Connection
	ds.platform = conf.Type
	ds.schemas = appConf.Data.DatastoreSchemas[alias]
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	logging.LogForComponent("sqlDatastoreTranslator").Infof("Configured SqlDatastoreTranslator [%s]", alias)
	return nil
}

func (ds *sqlDatastoreTranslator) Execute(ctx context.Context, query data.Node) (bool, error) {
	if !ds.configured {
		return false, errors.Errorf("SqlDatastoreTranslator: DatastoreTranslator was not configured! Please call Configure(). ")
	}
	logging.LogForComponent("sqlDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	// Translate query to into sql statement
	statement, params, err := ds.translatePrepared(query)
	if err != nil {
		return false, err
	}

	logging.LogForComponent("sqlDatastoreTranslator").Debugf("EXECUTING STATEMENT: ==================%s==================\nPARAMS: %+v", statement, params)

	return ds.executor.Execute(ctx, statement, params)
}

// nolint:gocyclo,gocritic
func (ds *sqlDatastoreTranslator) translatePrepared(input data.Node) (q string, params []interface{}, err error) {
	var query util.SStack
	var selects util.SStack
	var entities util.SStack
	var relations util.SStack
	var joins util.SStack

	var operands util.OpStack

	// Used for prepared statements
	var values []interface{}

	// Walk input
	err = input.Walk(func(q data.Node) error {
		switch v := q.(type) {
		case data.Union:
			// Expected stack:  top -> [Queries...]
			query = query.Push(strings.Join(selects, " UNION "))
			selects = selects[:0]
		case data.Query:
			// Expected stack: entities-top -> [singleEntity] relations-top -> [singleCondition]
			var (
				entity     string
				joinClause string
				condition  string
			)
			// Extract entity
			entities, entity = entities.Pop()
			// Extract joins
			for _, j := range joins {
				joinClause += j
			}
			// Extract condition
			if len(relations) > 0 {
				condition = relations[0]
				if len(relations) != 1 {
					return errors.Errorf("SqlDatastoreTranslator: Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", len(relations))
				}
			}

			//nolint:gosec
			selects = selects.Push(fmt.Sprintf("SELECT count(*) FROM %s%s%s", entity, joinClause, condition))
			joins = joins[:0]
			relations = relations[:0]
		case data.Link:
			// Expected stack: entities-top -> [entities]
			for _, entity := range entities {
				joins = joins.Push(fmt.Sprintf(", %s", entity))
			}
			entities = entities[:0]
		case data.Condition:
			// Expected stack: relations-top -> [singleRelation]
			if len(relations) > 0 {
				var rel string
				relations, rel = relations.Pop()
				//nolint:gosec
				relations = relations.Push(fmt.Sprintf(" WHERE %s", rel))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONDITION: relations |%+v <- TOP", relations)
			}
		case data.Disjunction:
			// Expected stack: relations-top -> [disjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(query, " OR ")))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("DISJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(relations, " AND ")))
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entities, entity = entities.Pop()
			operands.AppendToTop(fmt.Sprintf("%s.%s", entity, v.Name))
		case data.Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			operands, ops = operands.Pop()
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
			if len(operands) > 0 {
				// If we are in nested call -> push as operand
				operands.AppendToTop(nextRel)
			} else {
				// We reached root operation -> relation is processed
				relations = relations.Push(nextRel)
				logging.LogForComponent("sqlDatastoreTranslator").Debugf("RELATION DONE: relations |%+v <- TOP", relations)
			}
		case data.Operator:
			operands = operands.Push([]string{})
			operands.AppendToTop(v.String())
		case data.Entity:
			schema, entity, schemaError := ds.findSchemaForEntity(v.String())
			if schemaError != nil {
				return schemaError
			}
			if schema == "public" && ds.appConf.Data.Datastores[ds.alias].Type == "postgres" {
				// Special handle when datastore is postgres and schema is public
				entities = entities.Push(entity.Name)
			} else {
				// Normal case for all entities
				entities = entities.Push(fmt.Sprintf("%s.%s", schema, entity.Name))
			}
		case data.Constant:
			values = append(values, v.String())
			operands.AppendToTop(getPreparePlaceholderForPlatform(ds.platform, len(values)))
		default:
			return errors.Errorf("SqlDatastoreTranslator: Unexpected input: %T -> %+v", v, v)
		}
		return nil
	})
	if err != nil {
		logging.LogForComponent("sqlDatastoreTranslator: ").Debug(err)
	}
	return strings.Join(query, ""), values, err
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
