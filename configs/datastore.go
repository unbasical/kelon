package configs

type Datastore struct {
	Type       string
	Connection map[string]string
	Metadata   map[string]string
}

type EntitySchema struct {
	Entities []string
}

type DatastoreConfig struct {
	Datastores       map[string]*Datastore
	DatastoreSchemas map[string]map[string]*EntitySchema `yaml:"entity_schemas"`
}
