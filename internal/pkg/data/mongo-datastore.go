package data

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/util"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type mongoDatastore struct {
	appConf       *configs.AppConfig
	alias         string
	datastoreType string
	conn          map[string]string
	entityPaths   map[string]map[string][]string
	client        *mongo.Client
	callOps       map[string]func(args ...string) string
	configured    bool
}

type mongoQueryResult struct {
	err   error
	count int64
}

// Return a new data.Datastore which is able to connect to PostgreSQL and MySQL databases.
func NewMongoDatastore() data.Datastore {
	return &mongoDatastore{
		appConf:       nil,
		alias:         "",
		datastoreType: "Mongo-DB",
		callOps:       nil,
		configured:    false,
	}
}

func (ds *mongoDatastore) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	// Validate config
	conf, err := extractAndValidateDatastore(appConf, alias)
	if err != nil {
		return errors.Wrap(err, "MongoDatastore:")
	}
	if schemas, ok := appConf.Data.DatastoreSchemas[alias]; ok {
		if len(schemas) == 0 {
			return errors.Errorf("MongoDatastore: Datastore with alias [%s] has no schemas configured!", alias)
		}
	} else {
		return errors.Errorf("MongoDatastore: Datastore with alias [%s] has no entity-schema-mapping configured!", alias)
	}

	// Connect client
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(getConnectionStringForPlatform(conf.Type, conf.Connection))
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return errors.Wrap(err, "MongoDatastore: Error while connecting client")
	}

	// Ping mongodb for 60 seconds every 3 seconds
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = pingUntilReachable(alias, func() error {
		return client.Ping(ctx, readpref.Primary())
	})
	if err != nil {
		return errors.Wrap(err, "MongoDatastore:")
	}

	// Load call handlers
	operands, err := loadCallOperands(conf)
	if err != nil {
		return errors.Wrap(err, "MongoDatastore:")
	}
	ds.callOps = operands
	log.Infof("MongoDatastore [%s] laoded call operands", alias)

	// Load entity schemas
	ds.entityPaths = make(map[string]map[string][]string)
	for _, schema := range appConf.Data.DatastoreSchemas[alias] {
		paths := schema.GenerateEntityPaths()
		for k, v := range paths {
			ds.entityPaths[k] = v
		}
	}

	// Assign values
	ds.conn = conf.Connection
	ds.client = client
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
	queryResults := make([]mongoQueryResult, len(statements))

	// Execute all statements parallel and store resulting counts
	startTime := time.Now()
	var wg sync.WaitGroup
	writeIndex := 0
	wg.Add(len(queryResults))
	for collection, filterString := range statements {
		log.Debugf("EXECUTING Filter: ==================%s.find( %s )==================", collection, filterString)

		// Execute each of the resulting queries for each collection parallel
		go func(wait *sync.WaitGroup, index int, coll string, fString string) {
			defer wait.Done()

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
				queryResults[index] = mongoQueryResult{
					err:   searchErr,
					count: 0,
				}
				return
			}

			// Store result
			queryResults[index] = mongoQueryResult{
				err:   nil,
				count: count,
			}
		}(&wg, writeIndex, collection, filterString)

		// Increase write-index to avoid parallel write conflicts
		writeIndex++
	}

	// Wait till all queries returned
	wg.Wait()

	log.Debugf("RECEIVED RESULTS: %+v", queryResults)
	if ds.appConf.TelemetryProvider != nil {
		ds.appConf.TelemetryProvider.MeasureRemoteDependency(ds.alias, ds.datastoreType, time.Since(startTime), true)
	}
	decision := false
	for _, result := range queryResults {
		if result.err != nil {
			if ds.appConf.TelemetryProvider != nil {
				ds.appConf.TelemetryProvider.MeasureRemoteDependency(ds.alias, ds.datastoreType, time.Since(startTime), false)
			}
			return false, errors.Wrap(result.err, "MongoDB: Error while sending Queries to DB")
		}
		if result.count > 0 {
			log.Debugf("Result row with count %d found! -> ALLOWED", result.count)
			decision = true
		}
	}
	if !decision {
		log.Debugf("No resulting row with count > 0 found! -> DENIED")
	}
	return decision, nil
}

// nolint:gocyclo
func (ds mongoDatastore) translate(input *data.Node) map[string]string {
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
				collection := collection // pin!
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
					panic(fmt.Sprintf("MongoDatastore: Unable to find mapping for entity %q in collection %q", entity, collection))
				})

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
			// Reset entities because mongo does not join, but can only access directly nested elements!
			entities = entities[:0]
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
