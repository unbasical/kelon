package configs

// Configuration for the Datastore-mappings used by kelon to map incoming requests to datastores.
//
// This mapping is needed to set schemas for entities (coming from compiled regos) as well as using the datastore's alias as
// unknowns for OPA's partial evaluation api.
// The DatastoreSchema maps the datastore alias to a datastore schema to a slice of Entities which are contained in this schema.
type DatastoreConfig struct {
	Datastores       map[string]*Datastore
	DatastoreSchemas map[string]map[string]*EntitySchema `yaml:"entity_schemas"`
}

// Each datastore has a fixed Type (enum type) and variable connection-/metadata-properties
// Which should be validated and parsed by each data.Datastore separately.
type Datastore struct {
	Type       string
	Connection map[string]string
	Metadata   map[string]string
}

// List of entities of a schema
type EntitySchema struct {
	Entities []string
}
