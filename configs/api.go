package configs

type ApiConfig struct {
	Mappings []*DatastoreApiMapping `yaml:"apis"`
}

type DatastoreApiMapping struct {
	Prefix    string `yaml:"path-prefix"`
	Datastore string
	Mappings  []*ApiMapping
}

type ApiMapping struct {
	Path    string
	Package string
	Methods []string
	Queries []string
}
