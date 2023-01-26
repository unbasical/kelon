package extensions

import (
	"context"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
)

type Extension interface {
	Name() string
}

type ExtensionBuiltin interface {
	Extension
	Register(ctx context.Context, conf map[string]interface{}) error
}

type ExtensionDatastore interface {
	Extension
	Type() string
	Datastore() data.Datastore
}

type Factory interface {
	Configure(ctx context.Context, appConf *configs.AppConfig) error
	RegisterBuiltinFunctions(ctx context.Context) error
	MakeDatastore(ctx context.Context, dsType string) (data.Datastore, error)
}
