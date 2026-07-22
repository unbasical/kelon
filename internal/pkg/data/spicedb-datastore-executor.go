package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

// Connection keys of the spicedb datastore
const (
	spiceDBKeyEndpoint = "endpoint"
	spiceDBKeyHost     = "host"
	spiceDBKeyPort     = "port"
	spiceDBKeyScheme   = "scheme"
	spiceDBKeyToken    = "token"
)

// Metadata keys of the spicedb datastore
const (
	spiceDBMetaConsistency           = "consistency"
	spiceDBMetaRequestTimeoutSeconds = "requestTimeoutSeconds"
)

// Supported consistency modes
const (
	spiceDBConsistencyFullyConsistent = "fully_consistent"
	spiceDBConsistencyMinimizeLatency = "minimize_latency"
)

const spiceDBHasPermission = "PERMISSIONSHIP_HAS_PERMISSION"

// spiceDBObjectReference mirrors the v1 HTTP/JSON gateway's ObjectReference
type spiceDBObjectReference struct {
	ObjectType string `json:"objectType"`
	ObjectID   string `json:"objectId"`
}

// spiceDBSubjectReference mirrors the v1 HTTP/JSON gateway's SubjectReference
type spiceDBSubjectReference struct {
	Object spiceDBObjectReference `json:"object"`
}

// spiceDBConsistency mirrors the v1 HTTP/JSON gateway's Consistency
type spiceDBConsistency struct {
	FullyConsistent bool `json:"fullyConsistent,omitempty"`
	MinimizeLatency bool `json:"minimizeLatency,omitempty"`
}

// spiceDBCheckRequest mirrors the v1 HTTP/JSON gateway's CheckPermissionRequest
type spiceDBCheckRequest struct {
	Consistency spiceDBConsistency      `json:"consistency"`
	Resource    spiceDBObjectReference  `json:"resource"`
	Permission  string                  `json:"permission"`
	Subject     spiceDBSubjectReference `json:"subject"`
}

// spiceDBCheckResponse mirrors the relevant part of the v1 HTTP/JSON gateway's CheckPermissionResponse
type spiceDBCheckResponse struct {
	Permissionship string `json:"permissionship"`
}

type spiceDBQueryResult struct {
	err     error
	allowed bool
}

type spiceDBDatastoreExecutor struct {
	appConf     *configs.AppConfig
	endpoint    string
	token       string
	consistency string
	timeout     time.Duration
	client      *http.Client
	configured  bool
}

// NewSpiceDBDatastoreExecutor returns a new data.DatastoreExecutor which executes SpiceDB CheckPermission
// requests ([]SpiceDBCheck) against SpiceDB's v1 HTTP/JSON gateway.
func NewSpiceDBDatastoreExecutor() data.DatastoreExecutor {
	return &spiceDBDatastoreExecutor{
		appConf:    nil,
		client:     nil,
		configured: false,
	}
}

// Configure -- see data.DatastoreExecutor
func (ds *spiceDBDatastoreExecutor) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	if appConf == nil {
		return errors.Errorf("SpiceDBDatastoreExecutor: AppConfig not configured!")
	}
	if alias == "" {
		return errors.Errorf("SpiceDBDatastoreExecutor: Empty alias provided!")
	}
	conf, ok := appConf.Datastores[alias]
	if !ok {
		return errors.Errorf("SpiceDBDatastoreExecutor: No datastore with alias [%s] configured!", alias)
	}

	endpoint, err := extractSpiceDBEndpoint(alias, conf.Connection)
	if err != nil {
		return err
	}
	token, ok := conf.Connection[spiceDBKeyToken]
	if !ok || token == "" {
		return errors.Errorf("SpiceDBDatastoreExecutor: Field %s is missing in configured connection with alias %s!", spiceDBKeyToken, alias)
	}

	consistency, timeout, err := extractSpiceDBMetadata(conf.Metadata)
	if err != nil {
		return errors.Wrap(err, "SpiceDBDatastoreExecutor:")
	}

	ds.endpoint = endpoint
	ds.token = token
	ds.consistency = consistency
	ds.timeout = timeout
	ds.client = &http.Client{}

	// Ping SpiceDB for 60 seconds every 3 seconds
	if err := pingUntilReachable(alias, ds.ping); err != nil {
		return errors.Wrap(err, "SpiceDBDatastoreExecutor:")
	}

	ds.appConf = appConf
	ds.configured = true
	logging.LogForComponent("spiceDBDatastoreExecutor").Infof("Configured [%s]", alias)
	return nil
}

// ping treats any HTTP response (regardless of status) as reachable and only transport errors as unreachable
func (ds *spiceDBDatastoreExecutor) ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), ds.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ds.endpoint+"/v1/schema/read", strings.NewReader("{}"))
	if err != nil {
		return errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while building ping request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ds.token)

	resp, err := ds.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "SpiceDBDatastoreExecutor: SpiceDB is not reachable")
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return nil
}

// Execute -- see data.DatastoreExecutor
func (ds *spiceDBDatastoreExecutor) Execute(ctx context.Context, query data.DatastoreQuery) (bool, error) {
	checks, ok := query.Statement.([]SpiceDBCheck)
	if !ok {
		return false, errors.Errorf("Passed statement was not of type []SpiceDBCheck but of type: %T", query.Statement)
	}
	if len(checks) == 0 {
		return false, nil
	}

	queryResults := make([]spiceDBQueryResult, len(checks))

	// Execute all checks in parallel and store the results
	var wg sync.WaitGroup
	wg.Add(len(checks))
	for index, check := range checks {
		logging.LogForComponent("spiceDBDatastoreExecutor").Debugf("EXECUTING Check: ==================%+v==================", check)

		go func(index int, check SpiceDBCheck) {
			defer wg.Done()
			allowed, err := ds.checkPermission(ctx, check)
			queryResults[index] = spiceDBQueryResult{
				err:     err,
				allowed: allowed,
			}
		}(index, check)
	}

	// Wait till all checks returned
	wg.Wait()

	logging.LogForComponent("spiceDBDatastoreExecutor").Debugf("RECEIVED RESULTS: %+v", queryResults)
	decision := false
	for _, result := range queryResults {
		if result.err != nil {
			return false, errors.Wrap(result.err, "SpiceDB: Error while sending checks to SpiceDB")
		}
		if result.allowed {
			logging.LogForComponent("spiceDBDatastoreExecutor").Debugf("Check with permissionship %s found! -> ALLOWED", spiceDBHasPermission)
			decision = true
		}
	}
	if !decision {
		logging.LogForComponent("spiceDBDatastoreExecutor").Debugf("No check with permissionship %s found! -> DENIED", spiceDBHasPermission)
	}
	return decision, nil
}

// checkPermission fires a single CheckPermission request against SpiceDB's v1 HTTP/JSON gateway
func (ds *spiceDBDatastoreExecutor) checkPermission(ctx context.Context, check SpiceDBCheck) (bool, error) {
	objectType, objectID, err := splitSpiceDBRef(check.Object)
	if err != nil {
		return false, err
	}
	subjectType, subjectID, err := splitSpiceDBRef(check.Subject)
	if err != nil {
		return false, err
	}

	body, err := json.Marshal(spiceDBCheckRequest{
		Consistency: spiceDBConsistency{
			FullyConsistent: ds.consistency == spiceDBConsistencyFullyConsistent,
			MinimizeLatency: ds.consistency == spiceDBConsistencyMinimizeLatency,
		},
		Resource:   spiceDBObjectReference{ObjectType: objectType, ObjectID: objectID},
		Permission: check.Permission,
		Subject:    spiceDBSubjectReference{Object: spiceDBObjectReference{ObjectType: subjectType, ObjectID: subjectID}},
	})
	if err != nil {
		return false, errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while marshaling check request")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, ds.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, ds.endpoint+"/v1/permissions/check", bytes.NewReader(body))
	if err != nil {
		return false, errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while building check request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ds.token)

	resp, err := ds.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while executing check request")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while reading check response")
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.Errorf("SpiceDBDatastoreExecutor: SpiceDB returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var checkResponse spiceDBCheckResponse
	if err := json.Unmarshal(respBody, &checkResponse); err != nil {
		return false, errors.Wrap(err, "SpiceDBDatastoreExecutor: Error while parsing check response")
	}
	return checkResponse.Permissionship == spiceDBHasPermission, nil
}

// extractSpiceDBEndpoint builds the SpiceDB base URL from either the endpoint key or host/port(/scheme)
func extractSpiceDBEndpoint(alias string, conn map[string]string) (string, error) {
	if endpoint, ok := conn[spiceDBKeyEndpoint]; ok && endpoint != "" {
		return strings.TrimSuffix(endpoint, "/"), nil
	}

	host, hostOk := conn[spiceDBKeyHost]
	port, portOk := conn[spiceDBKeyPort]
	if !hostOk || !portOk || host == "" || port == "" {
		return "", errors.Errorf("SpiceDBDatastoreExecutor: Connection with alias %s must configure either %s or %s and %s!", alias, spiceDBKeyEndpoint, spiceDBKeyHost, spiceDBKeyPort)
	}
	scheme, ok := conn[spiceDBKeyScheme]
	if !ok || scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port), nil
}

// extractSpiceDBMetadata parses the optional metadata keys consistency and requestTimeoutSeconds
func extractSpiceDBMetadata(metadata map[string]string) (consistency string, timeout time.Duration, err error) {
	consistency = spiceDBConsistencyFullyConsistent
	timeout = 5 * time.Second

	if metadata == nil {
		return consistency, timeout, nil
	}
	if value, ok := metadata[spiceDBMetaConsistency]; ok {
		switch value {
		case spiceDBConsistencyFullyConsistent, spiceDBConsistencyMinimizeLatency:
			consistency = value
		default:
			return "", 0, errors.Errorf("Metadata field %s must be one of [%s, %s] but was %q!", spiceDBMetaConsistency, spiceDBConsistencyFullyConsistent, spiceDBConsistencyMinimizeLatency, value)
		}
	}
	if value, ok := metadata[spiceDBMetaRequestTimeoutSeconds]; ok {
		seconds, atoiErr := strconv.Atoi(value)
		if atoiErr != nil || seconds <= 0 {
			return "", 0, errors.Errorf("Metadata field %s must be a positive integer but was %q!", spiceDBMetaRequestTimeoutSeconds, value)
		}
		timeout = time.Duration(seconds) * time.Second
	}
	return consistency, timeout, nil
}

// splitSpiceDBRef splits a "<type>:<id>" reference on the first colon
func splitSpiceDBRef(ref string) (objectType, objectID string, err error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.Errorf("SpiceDBDatastoreExecutor: Malformed SpiceDB object reference %q! Expected format <type>:<id>.", ref)
	}
	return parts[0], parts[1], nil
}
