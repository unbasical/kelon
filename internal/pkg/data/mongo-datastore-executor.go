package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type mongoDatastoreExecuter struct {
	appConf *configs.AppConfig
	client  *mongo.Client
	conn    map[string]string
}

func NewMongoDatastoreExecuter() data.DatastoreExecutor {
	return &mongoDatastoreExecuter{
		appConf: nil,
		conn:    nil,
		client:  nil,
	}
}

func (ds *mongoDatastoreExecuter) Configure(appConf *configs.AppConfig, alias string) error {
	// Validate config
	conf, e := extractAndValidateDatastore(appConf, alias)
	if e != nil {
		return errors.Wrap(e, "mongoDatastoreExecuter:")
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
		pingErr := client.Ping(ctx, readpref.Primary())

		if pingErr != nil {
			_ = client.Disconnect(ctx)
			client, err = mongo.Connect(ctx, clientOptions)
			time.Sleep(1000 * time.Millisecond)
			return err
		}
		return nil
	})
	// Wait for mongo to be able to fulfill query requests
	if err != nil {
		return errors.Wrap(err, "MongoDatastoreExecutor:")
	}

	ds.client = client
	ds.conn = conf.Connection
	ds.appConf = appConf
	return nil
}

func (ds *mongoDatastoreExecuter) Execute(ctx context.Context, query data.DatastoreQuery) (bool, error) {
	mongoStatements, ok := query.Statement.(map[string]string)
	if !ok {
		return false, errors.Errorf("Passed statement was not of type map[string]string but of type: #{statement}")
	}

	queryResults := make([]mongoQueryResult, len(mongoStatements))

	// Execute all mongoStatements parallel and store resulting counts
	var wg sync.WaitGroup
	writeIndex := 0
	wg.Add(len(queryResults))
	entireQuery := ""
	for collection, filterString := range mongoStatements {
		entireQuery += fmt.Sprintf("%s->[%s]\n", collection, filterString)
		logging.LogForComponent("mongoDatastoreExecutor").Debugf("EXECUTING Filter: ==================%s.find( %s )==================", collection, filterString)

		// Execute each of the resulting queries for each collection parallel
		go func(wait *sync.WaitGroup, index int, coll string, fString string) {
			defer wait.Done()

			// Unmarshal generated json string
			var filter bson.M
			unmarshalErr := json.Unmarshal([]byte(fString), &filter)
			if unmarshalErr != nil {
				logging.LogForComponent("mongoDatastoreExecutor").Fatal("json.Unmarshal() ERROR:", unmarshalErr)
			}

			// Execute query
			collection := ds.client.Database(ds.conn[dbKey]).Collection(coll)
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			count, searchErr := collection.CountDocuments(timeoutCtx, filter)
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

	logging.LogForComponent("mongoDatastoreExecutor").Debugf("RECEIVED RESULTS: %+v", queryResults)
	decision := false
	for _, result := range queryResults {
		if result.err != nil {
			return false, errors.Wrap(result.err, "MongoDB: Error while sending Queries to DB")
		}
		if result.count > 0 {
			logging.LogForComponent("mongoDatastoreExecutor").Debugf("Result row with count %d found! -> ALLOWED", result.count)
			decision = true
		}
	}
	if !decision {
		logging.LogForComponent("mongoDatastoreExecutor").Debugf("No resulting row with count > 0 found! -> DENIED")
	}
	return decision, nil
}
