package configs

// Configuration for the API-mappings used by kelon to map incoming requests to rego packages.
type APIConfig struct {
	Mappings []*DatastoreAPIMapping `yaml:"apis"`
}

// API-mapping for one of the datastores defined in configs.DatastoreConfig.
//
// Each mapping has a type of 'mapping global' Prefix which should be appended to each Path of its Mappings.
// The prefix can be a regular expression.
type DatastoreAPIMapping struct {
	Prefix         string `yaml:"path-prefix"`
	Datastore      string
	Authentication *bool `yaml:",omitempty"`
	Authorization  *bool `yaml:",omitempty"`
	Mappings       []*APIMapping
}

// Mapping within a configs.DatastoreAPIMapping which holds all information that is needed to map an incoming
// request to a rego package.
// The Path can be a regular expression.
type APIMapping struct {
	Path    string
	Package string
	Methods []string
	Queries []string
}

func (d *DatastoreAPIMapping) setDefaults() {
	boolTrue := true

	if d.Authorization == nil {
		d.Authorization = &boolTrue
	}

	if d.Authentication == nil {
		d.Authentication = &boolTrue
	}
}
