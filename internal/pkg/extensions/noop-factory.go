package extensions

import (
	"context"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
	"github.com/unbasical/kelon/pkg/extensions"
)

type noopFactory struct{}

func NewNoopFactory() extensions.Factory {
	return &noopFactory{}
}

func (f *noopFactory) Configure(ctx context.Context, appConf *configs.AppConfig) error {
	return nil
}

func (f *noopFactory) RegisterBuiltinFunctions(ctx context.Context) error {
	return nil
}

func (f *noopFactory) MakeDatastore(ctx context.Context, dsType string) (data.Datastore, error) {
	return nil, errors.Errorf("no datastore with type %q found", dsType)
}
