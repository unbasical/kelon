package data

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Foundato/kelon/pkg/constants"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	// Import mysql dirver
	_ "github.com/go-sql-driver/mysql"
	// import postgres driver
	_ "github.com/lib/pq"
)

type sqlDatastore struct {
	appConf       *configs.AppConfig
	alias         string
	platform      string
	telemetryName string
	telemetryType string
	conn          map[string]string
	schemas       map[string]*configs.EntitySchema
	dbPool        *sql.DB
	callOps       map[string]func(args ...string) string
	configured    bool
}

// Return a new data.Datastore which is able to connect to PostgreSQL and MySQL databases.
func NewSQLDatastore() data.Datastore {
	return &sqlDatastore{
		appConf:       nil,
		alias:         "",
		telemetryName: "",
		callOps:       nil,
		configured:    false,
	}
}

func (ds *sqlDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Validate config
	conf, e := extractAndValidateDatastore(appConf, alias)
	if e != nil {
		return errors.Wrap(e, "SqlDatastore:")
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("SqlDatastore: Datastore with alias [%s] has no schemas configured!", alias)
		}

		for schemaName, schema := range schemas {
			if schema.HasNestedEntities() {
				return errors.Errorf("SqlDatastore: Schema %q in datastore with alias [%s] contains nested entities which is not supported by SQL-Datastores yet!", schemaName, alias)
			}
		}
	} else {
		return errors.Errorf("SqlDatastore: Datastore with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Init database connection pool
	db, err := sql.Open(conf.Type, getConnectionStringForPlatform(conf.Type, conf.Connection))
	if err != nil {
		return errors.Wrap(err, "SqlDatastore: Error while connecting to database")
	}

	// Configure metadata
	metadataError := ds.applyMetadataConfigs(alias, conf, appConf, db)
	if metadataError != nil {
		return errors.Wrap(err, "SqlDatastore: Error while configuring metadata")
	}

	// Ping database for 60 seconds every 3 seconds
	err = pingUntilReachable(alias, db.Ping)
	if err != nil {
		return errors.Wrap(err, "SqlDatastore:")
	}

	// Load call handlers
	operands, err := loadCallOperands(conf)
	if err != nil {
		return errors.Wrap(err, "SqlDatastore:")
	}
	ds.callOps = operands
	log.Infof("SqlDatastore [%s] laoded call operands", alias)

	// Assign values
	ds.conn = conf.Connection
	ds.platform = conf.Type
	ds.dbPool = db
	ds.schemas = appConf.Data.DatastoreSchemas[alias]
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	log.Infof("Configured SqlDatastore [%s]", alias)
	return nil
}

func (ds *sqlDatastore) applyMetadataConfigs(alias string, conf *configs.Datastore, appConf *configs.AppConfig, db *sql.DB) error {
	if conf.Metadata == nil {
		ds.telemetryName = constants.DefaultTelemetryName
		ds.telemetryType = "SQL"
		return nil
	}

	// Setup Datastore
	if maxOpenValue, ok := conf.Metadata[constants.MetaMaxOpenConnections]; ok {
		maxOpen, err := strconv.Atoi(maxOpenValue)
		if err != nil {
			return errors.Wrap(err, "SqlDatastore: Error while setting maxOpenConnections")
		}
		db.SetMaxOpenConns(maxOpen)
	}
	if maxIdleValue, ok := conf.Metadata[constants.MetaMaxIdleConnections]; ok {
		maxIdle, err := strconv.Atoi(maxIdleValue)
		if err != nil {
			return errors.Wrap(err, "SqlDatastore: Error while setting maxIdleConnections")
		}
		db.SetMaxIdleConns(maxIdle)
	}
	if maxLifetimeSecondsValue, ok := conf.Metadata[constants.MetaConnectionMaxLifetimeSeconds]; ok {
		maxLifetimeSeconds, err := strconv.Atoi(maxLifetimeSecondsValue)
		if err != nil {
			return errors.Wrap(err, "SqlDatastore: Error while setting connectionMaxLifetimeSeconds")
		}
		db.SetConnMaxLifetime(time.Second * time.Duration(maxLifetimeSeconds))
	}

	// Setup Telemetry
	if appConf.TelemetryProvider != nil {
		if telemetryName, ok := conf.Metadata[constants.MetaTelemetryName]; ok {
			ds.telemetryName = telemetryName
		} else {
			ds.telemetryName = alias
		}

		if telemetryType, ok := conf.Metadata[constants.MetaTelemetryType]; ok {
			ds.telemetryType = telemetryType
		} else {
			ds.telemetryType = conf.Type
		}
	}
	return nil
}

func (ds sqlDatastore) Execute(query *data.Node) (bool, error) {
	if !ds.configured {
		return false, errors.New("SqlDatastore was not configured! Please call Configure(). ")
	}
	log.Debugf("TRANSLATING QUERY: ==================%+v==================", (*query).String())

	// Translate query to into sql statement
	statement, params := ds.translatePrepared(query)
	log.Debugf("EXECUTING STATEMENT: ==================%s==================\nPARAMS: %+v", statement, params)

	startTime := time.Now()
	rows, err := ds.dbPool.Query(statement, params...)
	if err != nil {
		if ds.appConf.TelemetryProvider != nil {
			ds.appConf.TelemetryProvider.MeasureRemoteDependency(ds.telemetryName, ds.telemetryType, time.Since(startTime), statement, false)
		}
		return false, errors.Wrap(err, "SqlDatastore: Error while executing statement")
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Panic("Unable to close Result-Set!")
		}
	}()

	result := false
	for rows.Next() {
		var count int
		if err := rows.Scan(&count); err != nil {
			return false, errors.Wrap(err, "SqlDatastore: Unable to read result")
		}
		if count > 0 {
			log.Debugf("Result row with count %d found! -> ALLOWED", count)
			result = true
			break
		}
	}

	if !result {
		log.Debugf("No resulting row with count > 0 found! -> DENIED")
	}
	if ds.appConf.TelemetryProvider != nil {
		ds.appConf.TelemetryProvider.MeasureRemoteDependency(ds.telemetryName, ds.telemetryType, time.Since(startTime), statement, true)
	}
	return result, nil
}

// nolint:gocyclo
func (ds sqlDatastore) translatePrepared(input *data.Node) (string, []interface{}) {
	var query util.SStack
	var selects util.SStack
	var entities util.SStack
	var relations util.SStack
	var joins util.SStack

	var operands util.OpStack

	// Used for prepared statements
	var values []interface{}

	// Walk input
	(*input).Walk(func(q data.Node) {
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
					log.Errorf("Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", len(relations))
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
				log.Debugf("CONDITION: relations |%+v <- TOP", relations)
			}
		case data.Disjunction:
			// Expected stack: relations-top -> [disjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(query, " OR ")))
				log.Debugf("DISJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(relations, " AND ")))
				log.Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
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
				log.Debugln("NEW FUNCTION CALL")
				nextRel = sqlCallOp(ops[1:]...)
			} else {
				log.Panic(fmt.Sprintf("Datastores: Operator [%s] is not supported!", op))
			}
			if len(operands) > 0 {
				// If we are in nested call -> push as operand
				operands.AppendToTop(nextRel)
			} else {
				// We reached root operation -> relation is processed
				relations = relations.Push(nextRel)
				log.Debugf("RELATION DONE: relations |%+v <- TOP", relations)
			}
		case data.Operator:
			operands = operands.Push([]string{})
			operands.AppendToTop(v.String())
		case data.Entity:
			schema, entity := ds.findSchemaForEntity(v.String())
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
			log.Warnf("SqlDatastore: Unexpected input: %T -> %+v", v, v)
		}
	})

	return strings.Join(query, ""), values
}

func (ds sqlDatastore) findSchemaForEntity(search string) (string, *configs.Entity) {
	// Find custom mapping
	for schema, es := range ds.schemas {
		if found, entity := es.ContainsEntity(search); found {
			return schema, entity
		}
	}
	log.Panic(fmt.Sprintf("No schema found for entity %s in datastore with alias %s", search, ds.alias))
	return "", &configs.Entity{}
}
