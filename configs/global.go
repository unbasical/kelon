package configs

import "github.com/pkg/errors"

// Global holds the global configuration for the application.
type Global struct {
	Input Input `yaml:"input"`
}

// Input holds input related configuration, such as global header to input mappings.
type Input struct {
	HeaderMapping []*HeaderMapping `yaml:"header-mapping"`
}

// HeaderMapping is a simple struct to hold a key-value pair for headers that should be included in the request.
// If Name is not set, the header will be included as is, otherwise the header will be included as the specified name.
type HeaderMapping struct {
	Name  string `yaml:"name"`
	Alias string `yaml:"alias"`
}

func (g *Global) Validate() error {
	return g.Input.Validate()
}

func (i *Input) Validate() error {
	// Validate include header mappings
	headerCache := make(map[string]struct{})
	for _, header := range i.HeaderMapping {
		if header.Name == "" {
			return errors.Errorf("Empty header in include-header")
		}

		// If no target name is set, use the header as is
		if header.Alias == "" {
			header.Alias = header.Name
		}

		// check for duplicates
		if _, ok := headerCache[header.Name]; ok {
			return errors.Errorf("Duplicate header alias %q in include-header", header.Alias)
		}
		headerCache[header.Alias] = struct{}{}
	}

	return nil
}
