package configs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unbasical/kelon/configs"
)

//nolint:gochecknoglobals,gocritic
var wantedExternalConfig = configs.ExternalConfig{
	Global: configs.Global{
		Input: configs.Input{
			HeaderMapping: []*configs.HeaderMapping{
				{
					Name:  "Foo",
					Alias: "Foo",
				},
				{
					Name:  "Bar",
					Alias: "Baz",
				},
			},
		},
	},
	APIMappings: []*configs.DatastoreAPIMapping{
		{
			Prefix:         "/api",
			Datastores:     []string{"mysql"},
			Authorization:  &boolFalse,
			Authentication: &boolTrue,
			Mappings: []*configs.APIMapping{
				{
					Path:    "/.*",
					Package: "default",
					Methods: nil,
					Queries: nil,
				},
				{
					Path:    "/articles",
					Package: "articles",
					Methods: []string{"POST"},
					Queries: nil,
				},
				{
					Path:    "/articles",
					Package: "articles",
					Methods: []string{"GET"},
					Queries: []string{"author"},
				},
			},
		},
	},
	Datastores: map[string]*configs.Datastore{
		"mysql": {
			Type: "mysql",
			Connection: map[string]string{
				"host":     "localhost",
				"port":     "5432",
				"database": "mysql",
				"user":     "mysql",
				"password": "SuperSecure",
			},
			Metadata: map[string]string{
				"default_schema": "default",
			},
		},
		"local-json": {
			Type: "file",
			Connection: map[string]string{
				"location": "./data/local-data.json",
			},
			Metadata: map[string]string{
				"in_memory": "true",
			},
		},
	},
	DatastoreSchemas: map[string]map[string]*configs.EntitySchema{
		"mysql": {
			"appstore": {
				Entities: []*configs.Entity{
					{
						Name: "users",
					},
					{
						Name:  "user_followers",
						Alias: "followers",
					},
				},
			},
		},
	},
	OPA: struct{}{},
}

//nolint:gochecknoglobals,gocritic
var boolFalse = false

//nolint:gochecknoglobals,gocritic
var boolTrue = true

func TestLoadConfigFromFile(t *testing.T) {
	result, err := configs.FileConfigLoader{
		FilePath: "./testdata/valid.yml",
	}.Load()

	if err != nil {
		t.Errorf("Unexpected error while parsing config: %s", err)
	}

	assert.Equal(t, &wantedExternalConfig, result)
}

func TestLoadNotExistingDatastoreFile(t *testing.T) {
	_, err := configs.FileConfigLoader{
		FilePath: "./config-not-existing.yml",
	}.Load()
	assert.EqualError(t, err, "open ./config-not-existing.yml: no such file or directory")
}

func TestLoadAmbiguousEntitiesDatastoreFile(t *testing.T) {
	_, err := configs.FileConfigLoader{
		FilePath: "./testdata/datastore_ambiguous_entities.yml",
	}.Load()
	assert.EqualError(t, err, "loaded invalid configuration: The entity \"mysql.appstore.irrelevant\" collides with entity \"mysql.appstore.users\"!")
}

func TestLoadAmbiguousNestedEntitiesDatastoreFile(t *testing.T) {
	_, err := configs.FileConfigLoader{
		FilePath: "./testdata/datastore_ambiguous_nested_entities.yml",
	}.Load()
	assert.EqualError(t, err, "loaded invalid configuration: Found ambiguous nested entities in datastore \"mysql\" schema \"appstore\": The entity with name \"b\" collides with entity \"a\" inside path \"level1\"!")
}

func TestLoadApiWithoutDatastores(t *testing.T) {
	_, err := configs.FileConfigLoader{
		FilePath: "./testdata/api_no_datastores.yml",
	}.Load()

	assert.NoError(t, err)
}

func TestLoadAmbiguousEntitiesCausedByAPIMapping(t *testing.T) {
	_, err := configs.FileConfigLoader{
		FilePath: "./testdata/datastore_ambiguous_combined.yml",
	}.Load()

	assert.EqualError(t, err, "loaded invalid configuration: The entity \"pg.appstore.user_followers\" collides with entity \"mysql.appstore.followers\"!")
}
