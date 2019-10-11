package configs

// Configuration for the API-mappings used by kelon to map incoming requests to rego packages.
type ApiConfig struct {
	Mappings []*DatastoreApiMapping `yaml:"apis"`
}

// API-mapping for one of the datastores defined in configs.DatastoreConfig.
//
// Each mapping has a type of 'mapping global' Prefix which should be appended to each Path of its Mappings.
// The prefix can be a regular expression.
type DatastoreApiMapping struct {
	Prefix    string `yaml:"path-prefix"`
	Datastore string
	Mappings  []*ApiMapping
}

// Mapping within a configs.DatastoreApiMapping which holds all information that is needed to map an incoming
// request to a rego package.
// The Path can be a regular expression.
type ApiMapping struct {
	Path    string
	Package string
	Methods []string
	Queries []string
}
