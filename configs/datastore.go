package configs

import (
	"github.com/unbasical/kelon/pkg/constants/logging"
)

// DatastoreSchemas maps the datastore alias to a datastore schema to a slice of Entities which are contained in this schema.
type DatastoreSchemas = map[string]map[string]*EntitySchema

// Datastore has a fixed Type (enum type) and variable connection-/metadata-properties
// Which should be validated and parsed by each data.Datastore separately.
type Datastore struct {
	Type       string
	Connection map[string]string
	Metadata   map[string]string
}

// EntitySchema contains a List of entities of a schema
type EntitySchema struct {
	Entities []*Entity
}

// Entity inside a schema
type Entity struct {
	Name     string
	Alias    string
	Entities []*Entity
}

// ContainsEntity checks if an entity is contained inside a schema.
// The entity is searched by its alias which is either its name, or a specified alias!
//
// Returns a boolean indicating if the entity was found and the entity itself.
func (schema EntitySchema) ContainsEntity(search string) (bool, *Entity) {
	// Find custom mapping
	for _, entity := range schema.Entities {
		mapped := entity.getMappedName()
		if search == mapped {
			return true, entity
		}
	}
	return false, nil
}

// HasNestedEntities returns true if the schema contains entities with nested entities
func (schema EntitySchema) HasNestedEntities() bool {
	// Find custom mapping
	for _, entity := range schema.Entities {
		if len(entity.Entities) > 0 {
			return true
		}
	}
	return false
}

// GenerateEntityPaths returns search structure for fast finding of paths from a source to a destination
// returned map of maps has the semantic pathBegin -> pathEnd -> path
func (schema EntitySchema) GenerateEntityPaths() map[string]map[string][]string {
	result := make(map[string]map[string][]string)
	for _, entity := range schema.Entities {
		crawlEntityPaths(entity, entity, []string{}, result)
	}
	return result
}

func crawlEntityPaths(start, curr *Entity, pathHistory []string, path map[string]map[string][]string) {
	if start == nil {
		logging.LogForComponent("datastore").Panic("Cannot crawl start which is nil!")
		return
	}
	if curr == nil {
		logging.LogForComponent("datastore").Panic("Curr mustn't be nil!")
		return
	}

	if path[start.getMappedName()] == nil {
		path[start.getMappedName()] = make(map[string][]string)
	}
	pathHistory = append(pathHistory, curr.Name)
	path[start.getMappedName()][curr.getMappedName()] = append(pathHistory[:0:0], pathHistory...)

	// Exits if there are no more children
	for _, child := range curr.Entities {
		// Recursion
		crawlEntityPaths(start, child, append(pathHistory[:0:0], pathHistory...), path)
	}
}

func (entity Entity) getMappedName() string {
	if entity.Alias != "" {
		return entity.Alias
	}
	return entity.Name
}
