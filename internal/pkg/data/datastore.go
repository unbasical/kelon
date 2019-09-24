package data

import (
	"database/sql"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/util"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"strings"
)

type Datastore interface {
	Configure(appConf *configs.AppConfig, alias string) error
	Execute(query *Node) (bool, error)
}

type mysqlDatastore struct {
	appConf       *configs.AppConfig
	alias         string
	conn          map[string]string
	schemas       map[string]*configs.EntitySchema
	defaultSchema string
	dbPool        *sql.DB
	configured    bool
}

var relationOperators = map[string]string{
	"eq":    "=",
	"equal": "=",
	"neq":   "!=",
	"lt":    "<",
	"gt":    ">",
	"lte":   "<=",
	"gte":   ">=",
}

var callOperators = map[string]func(args ...string) string{}

var (
	hostKey          = "host"
	portKey          = "port"
	dbKey            = "database"
	userKey          = "user"
	pwKey            = "password"
	defaultSchemaKey = "default_schema"
)

func NewMysqlDatastore() Datastore {
	return &mysqlDatastore{
		appConf:    nil,
		alias:      "",
		configured: false,
	}
}

func (ds *mysqlDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	if appConf == nil {
		return errors.New("MySqlDatastore: AppConfig not configured! ")
	}
	if alias == "" {
		return errors.New("MySqlDatastore: Empty alias provided! ")
	}

	// Validate configuration
	conf, ok := appConf.Data.Datastores[alias]
	if !ok {
		return errors.Errorf("MySqlDatastore: No datastore with alias [%s] configured!", alias)
	}
	if strings.ToLower(conf.Type) != "mysql" {
		return errors.Errorf("MySqlDatastore: Datastore with alias [%s] is not of type mysql! Type is: %s", alias, strings.ToLower(conf.Type))
	}
	if err := validateConnection(alias, conf.Connection); err != nil {
		return err
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("MySqlDatastore: Datastore with alias [%s] has no schemas configured!", alias)
		}
	} else {
		return errors.Errorf("MySqlDatastore: Datastore with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Extract metadata
	if s, ok := conf.Metadata[defaultSchemaKey]; ok {
		ds.defaultSchema = s
	}

	// Init database connection pool
	connString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", conf.Connection[userKey], conf.Connection[pwKey], conf.Connection[hostKey], conf.Connection[portKey], conf.Connection[dbKey])
	db, err := sql.Open("mysql", connString)
	if err != nil {
		return errors.Wrap(err, "MySqlDatastore: Error while connecting to database")
	}
	if err = db.Ping(); err != nil {
		return errors.Wrap(err, "MySqlDatastore: Unable to ping database")
	}

	// Load call handlers
	for _, handler := range MySqlCallHandlers {
		callOperators[handler.Handles()] = handler.Map
	}

	// Assign values
	ds.conn = conf.Connection
	ds.dbPool = db
	ds.schemas = appConf.Data.DatastoreSchemas[alias]
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	log.Infoln("Configured MySqlDatastore")
	return nil
}

func (ds mysqlDatastore) Execute(query *Node) (bool, error) {
	if !ds.configured {
		return false, errors.New("MySqlDatastore was not configured! Please call Configure(). ")
	}
	log.Debugf("TRANSLATING QUERY: ==================\n%+v\n ==================", (*query).String())

	// Translate query to into sql statement
	statement, err := ds.translate(query)
	if err != nil {
		return false, errors.New("MySqlDatastore: Unable to translate Query!")
	}
	log.Debugf("EXECUTING STATEMENT: ==================\n%s\n ==================", statement)

	rows, err := ds.dbPool.Query(statement)
	if err != nil {
		return false, errors.Wrap(err, "MySqlDatastore: Error while executing statement")
	}
	defer func() {
		if err := rows.Close(); err != nil {
			panic("Unable to close Result-Set!")
		}
	}()

	for rows.Next() {
		var count int
		if err := rows.Scan(&count); err != nil {
			return false, errors.Wrap(err, "MySqlDatastore: Unable to read result")
		}
		if count > 0 {
			log.Infof("Result row with count %d found! -> ALLOWED\n", count)
			return true, nil
		}
	}

	log.Infof("No resulting row with count > 0 found! -> DENIED")
	return false, nil
}

func (ds mysqlDatastore) translate(input *Node) (string, error) {
	var query util.SStack
	var selects util.SStack
	var entities util.SStack
	var relations util.SStack
	var joins util.SStack

	var operands util.OpStack

	// Walk input
	(*input).Walk(func(q Node) {
		switch v := q.(type) {
		case Union:
			// Expected stack:  top -> [Queries...]
			query = query.Push(strings.Join(selects, "\nUNION\n"))
			selects = selects[:0]
		case Query:
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
					log.Errorf("Error while building Query: Too many relations left to build 1 condition! len(relations) = %d\n", len(relations))
				}
			}

			selects = selects.Push(fmt.Sprintf("SELECT count(*) FROM %s%s%s", entity, joinClause, condition))
			joins = joins[:0]
			relations = relations[:0]
		case Link:
			// Expected stack: entities-top -> [entities] relations-top -> [relations]
			if len(entities) != len(relations) {
				log.Errorf("Error while creating Link: Entities and relations are not balanced! Lengths are Entities[%d:%d]Relations\n", len(entities), len(relations))
			}
			for i, entity := range entities {
				joins = joins.Push(fmt.Sprintf("\n\tINNER JOIN %s \n\t\tON %s", entity, strings.Replace(relations[i], "WHERE", "", 1)))
			}
			entities = entities[:0]
			relations = relations[:0]
		case Condition:
			// Expected stack: relations-top -> [singleRelation]
			if len(relations) > 0 {
				var rel string
				relations, rel = relations.Pop()
				relations = relations.Push(fmt.Sprintf("\n\tWHERE \n\t\t%s", rel))
				log.Debugf("CONDITION: relations |%+v <- TOP\n", relations)
			}
		case Disjunction:
			// Expected stack: relations-top -> [disjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(query, "\n\t\tOR ")))
				log.Debugf("DISJUNCTION: relations |%+v <- TOP\n", relations)
			}
		case Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(relations, "\n\t\tAND ")))
				log.Debugf("CONJUNCTION: relations |%+v <- TOP\n", relations)
			}
		case Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entities, entity = entities.Pop()
			operands.AppendToTop(fmt.Sprintf("%s.%s", entity, v.Name))
		case Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			operands, ops = operands.Pop()
			op := ops[0]

			// Handle Call
			var nextRel string
			if sqlRelOp, ok := relationOperators[op]; ok {
				// Expected stack:  top -> [rhs, lhs, call-op]
				log.Debugln("NEW RELATION")
				nextRel = fmt.Sprintf("%s %s %s", ops[1], sqlRelOp, ops[2])
			} else if sqlCallOp, ok := callOperators[op]; ok {
				// Expected stack:  top -> [args..., call-op]
				log.Debugln("NEW FUNCTION CALL")
				nextRel = sqlCallOp(ops[1:]...)
			} else {
				panic(fmt.Sprintf("Datastores: Operator [%s] is not supported!", op))
			}

			if len(operands) > 0 {
				// If we are in nested call -> push as operand
				operands.AppendToTop(nextRel)
			} else {
				// We reached root operation -> relation is processed
				relations = relations.Push(nextRel)
				log.Debugf("RELATION DONE: relations |%+v <- TOP\n", relations)
			}
		case Operator:
			operands = operands.Push([]string{})
			operands.AppendToTop(v.String())
		case Entity:
			entity := v.String()
			entities = entities.Push(fmt.Sprintf("%s.%s", ds.findSchemaForEntity(entity), entity))
		case Constant:
			operands.AppendToTop(fmt.Sprintf("'%s'", v.String()))
		default:
			log.Warnf("Mysql datastore: Unexpected input: %T -> %+v\n", v, v)
		}
	})

	return strings.Join(query, "\n"), nil
}

func (ds mysqlDatastore) findSchemaForEntity(search string) string {
	// Find custom mapping
	for schema, es := range ds.schemas {
		for _, entity := range es.Entities {
			if search == entity {
				return schema
			}
		}
	}

	// Assign default schema if exists
	if ds.defaultSchema != "" {
		return ds.defaultSchema
	}
	return ""
}

func validateConnection(alias string, conn map[string]string) error {
	if _, ok := conn[hostKey]; !ok {
		return errors.Errorf("MySqlDatastore: Field %s is missing in configured connection with alias %s!", hostKey, alias)
	}
	if _, ok := conn[portKey]; !ok {
		return errors.Errorf("MySqlDatastore: Field %s is missing in configured connection with alias %s!", portKey, alias)
	}
	if _, ok := conn[dbKey]; !ok {
		return errors.Errorf("MySqlDatastore: Field %s is missing in configured connection with alias %s!", dbKey, alias)
	}
	if _, ok := conn[userKey]; !ok {
		return errors.Errorf("MySqlDatastore: Field %s is missing in configured connection with alias %s!", userKey, alias)
	}
	if _, ok := conn[pwKey]; !ok {
		return errors.Errorf("MySqlDatastore: Field %s is missing in configured connection with alias %s!", pwKey, alias)
	}
	return nil
}
