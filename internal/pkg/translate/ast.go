package translate

import "github.com/Foundato/kelon/internal/pkg/datastore"

type AstTranslatorConfig struct {
	Datastore datastore.Datastore
}

type AstTranslator interface {
}
