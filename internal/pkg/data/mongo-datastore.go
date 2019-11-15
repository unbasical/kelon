package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Foundato/kelon/pkg/data"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoDatastore struct {
	appConf    *configs.AppConfig
	alias      string
	conn       map[string]string
	schemas    map[string]*configs.EntitySchema
	client     *mongo.Client
	callOps    map[string]func(args ...string) string
	configured bool
}

// Return a new data.Datastore which is able to connect to PostgreSQL and MySQL databases.
func NewMongoDatastore() data.Datastore {
	return &mongoDatastore{
		appConf:    nil,
		alias:      "",
		callOps:    nil,
		configured: false,
	}
}

func (ds *mongoDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	if appConf == nil {
		return errors.New("MongoDatastore: AppConfig not configured! ")
	}
	if alias == "" {
		return errors.New("MongoDatastore: Empty alias provided! ")
	}

	// Validate configuration
	conf, ok := appConf.Data.Datastores[alias]
	if !ok {
		return errors.Errorf("MongoDatastore: No datastore with alias [%s] configured!", alias)
	}
	if strings.ToLower(conf.Type) == "" {
		return errors.Errorf("MongoDatastore: Alias of datastore is empty! Must be one of %+v!", sql.Drivers())
	}
	if err := validateConnection(alias, conf.Connection); err != nil {
		return err
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("MongoDatastore: Datastore with alias [%s] has no schemas configured!", alias)
		}
	} else {
		return errors.Errorf("MongoDatastore: Datastore with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Connect client
	conn := conf.Connection
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%s/%s", conn[userKey], conn[pwKey], conn[hostKey], conn[portKey], conn[dbKey]))
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return errors.Wrap(err, "MongoDatastore: Error while connecting client")
	}

	// Ping mongodb for 60 seconds every 3 seconds
	var pingFailure error
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 20; i++ {
		if pingFailure = client.Ping(ctx, readpref.Primary()); pingFailure == nil {
			// Ping succeeded
			break
		}
		log.Infof("Waiting for [%s] to be reachable...", alias)
		<-time.After(3 * time.Second)
	}
	if pingFailure != nil {
		return errors.Wrap(err, "MongoDatastore: Unable to ping database")
	}

	// Load call handlers
	callOpsFile := fmt.Sprintf("./call-operands/%s.yml", strings.ToLower(conf.Type))
	handlers, err := LoadDatastoreCallOpsFile(callOpsFile)
	if err != nil {
		return errors.Wrap(err, "MongoDatastore: Unable to load call operands as handlers")
	}
	log.Infof("MongoDatastore [%s] laoded call operands [%s]", alias, callOpsFile)

	ds.callOps = map[string]func(args ...string) string{}
	for _, handler := range handlers {
		ds.callOps[handler.Handles()] = handler.Map
	}

	// Assign values
	ds.conn = conf.Connection
	ds.client = client
	ds.schemas = appConf.Data.DatastoreSchemas[alias]
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	log.Infof("Configured MongoDatastore [%s]", alias)
	return nil
}

func (ds mongoDatastore) Execute(query *data.Node) (bool, error) {
	if !ds.configured {
		return false, errors.New("MongoDatastore was not configured! Please call Configure(). ")
	}
	log.Debugf("TRANSLATING QUERY: ==================%+v==================", (*query).String())

	// Translate to map: collection -> filter
	statements := ds.translate(query)
	queryResults := make([]int64, len(statements))

	// Execute all statements parallel and store resulting counts
	var wg sync.WaitGroup
	writeIndex := 0
	wg.Add(len(queryResults))
	for collection, filterString := range statements {
		log.Debugf("EXECUTING Filter: ==================%s.find( %s )==================", collection, filterString)

		// Execute each of the resulting queries for each collection parallel
		go func(wait *sync.WaitGroup, index int, coll string, fString string) {
			// Unmarshal generated json string
			var filter bson.M
			unmarshalErr := json.Unmarshal([]byte(fString), &filter)
			if unmarshalErr != nil {
				log.Fatal("json.Unmarshal() ERROR:", unmarshalErr)
			}

			// Execute query
			collection := ds.client.Database(ds.conn[dbKey]).Collection(coll)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			count, searchErr := collection.CountDocuments(ctx, filter)
			if searchErr != nil {
				panic(searchErr)
			}

			// Store result
			queryResults[index] = count
			wait.Done()
		}(&wg, writeIndex, collection, filterString)

		// Increase write-index to avoid parallel write conflicts
		writeIndex++
	}

	// Wait till all queries returned
	wg.Wait()

	// Recover from any occurred error during transaction
	if err := recover(); err != nil {
		return false, errors.Errorf("Error while executing MongoDB query: %+v", err)
	}

	log.Debugf("RECEIVED RESULTS: %+v", queryResults)
	for _, count := range queryResults {
		if count > 0 {
			log.Infof("Result row with count %d found! -> ALLOWED", count)
			return true, nil
		}
	}
	log.Infof("No resulting row with count > 0 found! -> DENIED")
	return false, nil
}

func (ds mongoDatastore) translate(input *data.Node) map[string]string {
	type colFilter struct {
		collection string
		filter     string
	}

	entityMarker := "#ENTITY ->"
	result := make(map[string]string)
	filtersByCollection := make(map[string][]string)
	var filters []colFilter
	var entities util.SStack
	var relations util.SStack
	var operands util.OpStack

	// Walk input
	(*input).Walk(func(q data.Node) {
		switch v := q.(type) {
		case data.Union:
			// Sort collection filters by collection
			for _, colF := range filters {
				if _, exists := filtersByCollection[colF.collection]; exists {
					// Append filter to existing entry
					filtersByCollection[colF.collection] = append(filtersByCollection[colF.collection], colF.filter)
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

				// TODO handle access to nested entities
				// TODO handle access to array entities
				search := fmt.Sprintf("%s\"%s.", entityMarker, collection)
				combinedFilterWithoutBaseEntity := strings.ReplaceAll(strings.ReplaceAll(combinedFilter, search, "\""), entityMarker, "")
				result[collection] = combinedFilterWithoutBaseEntity
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
					log.Errorf("Error while building Query: Too many relations left to build 1 condition! len(relations) = %d", len(relations))
				}
			}

			// Append new filter
			filters = append(filters, colFilter{
				collection: entity,
				filter:     condition,
			})
			relations = relations[:0]
		case data.Link:
			// Reset entities and relations because mongo does not join, but can only access directly nested elements!
			entities = entities[:0]
			relations = relations[:0]
		case data.Condition:
			// Skip condition
		case data.Conjunction:
			// Expected stack: relations-top -> [conjunctions ...]
			if len(relations) > 0 {
				relations = relations[:0].Push(fmt.Sprintf("{%s}", strings.Join(relations, ", ")))
				log.Debugf("CONJUNCTION: relations |%+v <- TOP", relations)
			}
		case data.Attribute:
			// Expected stack:  top -> [entity, ...]
			var entity string
			entities, entity = entities.Pop()
			operands.AppendToTop(fmt.Sprintf("%s\"%s.%s\"", entityMarker, entity, v.Name))
		case data.Call:
			// Expected stack:  top -> [args..., call-op]
			var ops []string
			operands, ops = operands.Pop()
			op := ops[0]

			// Sort call operands in case of eq operation
			// This has to be done because MongoDB maps equality to normal JSON-Attributes.
			if len(ops) == 3 {
				// Check if first operand is not an entity
				if !strings.HasPrefix(ops[1], entityMarker) {
					// Swap operands
					ops[1], ops[2] = ops[2], ops[1]
				}
			}

			// Handle Call
			var nextRel string
			if mongoCallOp, ok := ds.callOps[op]; ok {
				// Expected stack:  top -> [args..., call-op]
				log.Debugln("NEW FUNCTION CALL")
				nextRel = mongoCallOp(ops[1:]...)
			} else {
				panic(fmt.Sprintf("Datastores: Operator [%s] is not supported!", op))
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
			entities = entities.Push(v.String())
		case data.Constant:
			if v.IsNumeric {
				operands.AppendToTop(v.String())
			} else {
				operands.AppendToTop(fmt.Sprintf("\"%s\"", v.String()))
			}
		default:
			log.Warnf("MongoDatastore: Unexpected input: %T -> %+v", v, v)
		}
	})

	return result
}