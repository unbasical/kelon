package configs

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

//nolint:gochecknoglobals,gocritic
var boolTrue = true

// DatastoreAPIMapping holds the API-mappings for one of the datastores defined in configs.DatastoreConfig.
//
// Each mapping has a type of 'mapping global' Prefix which should be appended to each Path of its Mappings.
// The prefix can be a regular expression.
type DatastoreAPIMapping struct {
	Prefix         string   `yaml:"path-prefix"`
	Datastores     []string `yaml:"datastores,omitempty"`
	Authentication *bool    `yaml:",omitempty"`
	Authorization  *bool    `yaml:",omitempty"`
	Mappings       []*APIMapping
}

// APIMapping within a configs.DatastoreAPIMapping which holds all information that is needed to map an incoming
// request to a rego package.
// The Path can be a regular expression.
type APIMapping struct {
	Path    string
	Package string
	Methods []string
	Queries []string
}

func (m *DatastoreAPIMapping) Validate(schema DatastoreSchemas) error {
	duplicatesCache := make(map[string]struct {
		path   string
		entity *Entity
	})

	for _, dsAlias := range m.Datastores {
		dsSchemas, ok := schema[dsAlias]
		if !ok {
			return errors.Errorf("datastore with name [%s] not found in datastore config", dsAlias)
		}

		for schemaName, schema := range dsSchemas {
			for _, entity := range schema.Entities {
				// Search for duplicated entities inside all schemas for all datastores
				search := entity.getMappedName()
				if match, ok := duplicatesCache[search]; ok {
					collidingPath := fmt.Sprintf("%s.%s.%s", dsAlias, schemaName, entity.Name)
					return errors.Errorf("The entity %q collides with entity %q!", collidingPath, match.path)
				}
				duplicatesCache[search] = struct {
					path   string
					entity *Entity
				}{path: fmt.Sprintf("%s.%s.%s", dsAlias, schemaName, search), entity: entity}

				// Search for any ambiguity inside each level of nested entities
				if err := findEntityAmbiguity(*entity, []string{}); err != nil {
					return errors.Wrapf(err, "Found ambiguous nested entities in datastore %q schema %q", dsAlias, schemaName)
				}
			}
		}
	}

	return nil
}

func (m *DatastoreAPIMapping) Defaults() {
	if m.Authorization == nil {
		m.Authorization = &boolTrue
	}

	if m.Authentication == nil {
		m.Authentication = &boolTrue
	}
}

func findEntityAmbiguity(entity Entity, pathHistory []string) error {
	// Reached end of recursion
	if entity.Entities == nil {
		return nil
	}

	// Crawl all nested entities and check for ambiguity on each level
	duplicatesCache := make(map[string]*Entity)
	for _, child := range entity.Entities {
		search := child.getMappedName()
		if match, ok := duplicatesCache[search]; ok {
			return errors.Errorf("The entity with name %q collides with entity %q inside path %q!", child.Name, match.Name, strings.Join(pathHistory, " -> "))
		}
		duplicatesCache[search] = child

		// Descend one level in entity tree
		if err := findEntityAmbiguity(*child, append(pathHistory, search)); err != nil {
			return err
		}
	}
	return nil
}
