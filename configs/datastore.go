package configs

type Datastore struct {
	Type       string
	Connection map[string]string
	Metadata   map[string]string
}

type EntityLink struct {
	Column string
	Link   map[string]*EntityLinkTarget
}

type EntityLinkTarget struct {
	Column string
}

type EntitySchema struct {
	Entities []string
	Links    map[string]*[]EntityLink
}

type DatastoreConfig struct {
	Datastores       map[string]*Datastore
	DatastoreSchemas map[string]map[string]*EntitySchema `yaml:"entity_schemas"`
}
