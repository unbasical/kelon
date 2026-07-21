package data

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
)

// spiceDBTestServer stubs SpiceDB's v1 HTTP/JSON gateway. The check decision is derived
// from the resource objectId: "allowed" -> HAS_PERMISSION, "boom" -> HTTP 500, else NO_PERMISSION.
type spiceDBTestServer struct {
	server *httptest.Server

	mtx           sync.Mutex
	checkRequests []map[string]any
	authHeaders   []string
}

func newSpiceDBTestServer(t *testing.T) *spiceDBTestServer {
	t.Helper()
	ts := &spiceDBTestServer{}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/schema/read":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"schemaText": ""}`))
		case "/v1/permissions/check":
			var body map[string]any
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

			ts.mtx.Lock()
			ts.checkRequests = append(ts.checkRequests, body)
			ts.authHeaders = append(ts.authHeaders, r.Header.Get("Authorization"))
			ts.mtx.Unlock()

			resource, _ := body["resource"].(map[string]any)
			switch resource["objectId"] {
			case "allowed":
				_, _ = w.Write([]byte(`{"permissionship": "PERMISSIONSHIP_HAS_PERMISSION"}`))
			case "boom":
				http.Error(w, "internal error", http.StatusInternalServerError)
			default:
				_, _ = w.Write([]byte(`{"permissionship": "PERMISSIONSHIP_NO_PERMISSION"}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.server.Close)
	return ts
}

func newTestSpiceDBExecutor(t *testing.T, endpoint string, metadata map[string]string) data.DatastoreExecutor {
	t.Helper()
	appConf := &configs.AppConfig{
		ExternalConfig: configs.ExternalConfig{
			Datastores: map[string]*configs.Datastore{
				"spicedb": {
					Type:       data.TypeSpicedb,
					Connection: map[string]string{"endpoint": endpoint, "token": "test-token"},
					Metadata:   metadata,
				},
			},
		},
	}

	executor := NewSpiceDBDatastoreExecutor()
	assert.NoError(t, executor.Configure(appConf, "spicedb"))
	return executor
}

func Test_SpiceDBExecutor_RequestShapeAndAllow(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "asset:allowed", Permission: "view", Subject: "user:alice"},
	}})
	assert.NoError(t, err)
	assert.True(t, decision)

	assert.Len(t, ts.checkRequests, 1)
	assert.Equal(t, map[string]any{
		"consistency": map[string]any{"fullyConsistent": true},
		"resource":    map[string]any{"objectType": "asset", "objectId": "allowed"},
		"permission":  "view",
		"subject":     map[string]any{"object": map[string]any{"objectType": "user", "objectId": "alice"}},
	}, ts.checkRequests[0])
	assert.Equal(t, "Bearer test-token", ts.authHeaders[0])
}

func Test_SpiceDBExecutor_MinimizeLatencyConsistency(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{"consistency": "minimize_latency"})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "asset:denied", Permission: "view", Subject: "user:alice"},
	}})
	assert.NoError(t, err)
	assert.False(t, decision)

	assert.Len(t, ts.checkRequests, 1)
	assert.Equal(t, map[string]any{"minimizeLatency": true}, ts.checkRequests[0]["consistency"])
}

func Test_SpiceDBExecutor_OrSemantics(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "asset:denied", Permission: "view", Subject: "user:alice"},
		{Object: "asset:allowed", Permission: "view", Subject: "user:alice"},
	}})
	assert.NoError(t, err)
	assert.True(t, decision)
	assert.Len(t, ts.checkRequests, 2)
}

func Test_SpiceDBExecutor_AllDenied(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "asset:denied", Permission: "view", Subject: "user:alice"},
		{Object: "asset:denied-too", Permission: "view", Subject: "user:bob"},
	}})
	assert.NoError(t, err)
	assert.False(t, decision)
}

func Test_SpiceDBExecutor_HTTPErrorPropagates(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "asset:allowed", Permission: "view", Subject: "user:alice"},
		{Object: "asset:boom", Permission: "view", Subject: "user:alice"},
	}})
	assert.ErrorContains(t, err, "status 500")
	assert.False(t, decision)
}

func Test_SpiceDBExecutor_MalformedReference(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{
		{Object: "no-colon", Permission: "view", Subject: "user:alice"},
	}})
	assert.ErrorContains(t, err, "Malformed SpiceDB object reference")
	assert.False(t, decision)
}

func Test_SpiceDBExecutor_EmptyStatement(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: []SpiceDBCheck{}})
	assert.NoError(t, err)
	assert.False(t, decision)
	assert.Empty(t, ts.checkRequests)
}

func Test_SpiceDBExecutor_WrongStatementType(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	executor := newTestSpiceDBExecutor(t, ts.server.URL, map[string]string{})

	decision, err := executor.Execute(t.Context(), data.DatastoreQuery{Statement: "SELECT 1"})
	assert.ErrorContains(t, err, "was not of type []SpiceDBCheck")
	assert.False(t, decision)
}

func Test_SpiceDBExecutor_MissingToken(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	appConf := &configs.AppConfig{
		ExternalConfig: configs.ExternalConfig{
			Datastores: map[string]*configs.Datastore{
				"spicedb": {
					Type:       data.TypeSpicedb,
					Connection: map[string]string{"endpoint": ts.server.URL},
					Metadata:   map[string]string{},
				},
			},
		},
	}

	executor := NewSpiceDBDatastoreExecutor()
	assert.ErrorContains(t, executor.Configure(appConf, "spicedb"), "Field token is missing")
}

func Test_SpiceDBExecutor_InvalidConsistency(t *testing.T) {
	ts := newSpiceDBTestServer(t)
	appConf := &configs.AppConfig{
		ExternalConfig: configs.ExternalConfig{
			Datastores: map[string]*configs.Datastore{
				"spicedb": {
					Type:       data.TypeSpicedb,
					Connection: map[string]string{"endpoint": ts.server.URL, "token": "test-token"},
					Metadata:   map[string]string{"consistency": "eventually"},
				},
			},
		},
	}

	executor := NewSpiceDBDatastoreExecutor()
	assert.ErrorContains(t, executor.Configure(appConf, "spicedb"), "must be one of")
}
