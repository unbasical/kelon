package extensions

import (
	"context"
	"fmt"
	"os"
	"plugin"
	"strings"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"github.com/unbasical/kelon/pkg/extensions"
)

type defaultFactory struct {
	configured   bool
	extensionDir string
	appConf      *configs.AppConfig
	builtins     map[string]*extensions.ExtensionBuiltin   // ExtensionName -> Extension
	datastores   map[string]*extensions.ExtensionDatastore // ExtensionName.DatastoreType -> Extension
}

func NewExtensionFactory(extensionPath string) extensions.Factory {
	return &defaultFactory{
		configured:   false,
		extensionDir: normalizeDirPath(extensionPath),
		builtins:     map[string]*extensions.ExtensionBuiltin{},
		datastores:   map[string]*extensions.ExtensionDatastore{},
	}
}

func (f *defaultFactory) Configure(ctx context.Context, appConf *configs.AppConfig) error {
	f.appConf = appConf

	err := f.loadExtensionsFromDir(f.extensionDir)
	if err != nil {
		return err
	}

	f.configured = true
	return nil
}

func (f *defaultFactory) RegisterBuiltinFunctions(ctx context.Context) error {
	if !f.configured {
		return errors.New("extension factory not configured")
	}

	for _, builtinConf := range f.appConf.Builtins {
		extension, ok := f.builtins[builtinConf.Extension]
		if !ok {
			return errors.Errorf("no extension loaded with name %q", builtinConf.Extension)
		}

		err := (*extension).Register(ctx, builtinConf.Config)
		if err != nil {
			return errors.Wrapf(err, "error configuring extension %s", builtinConf.Extension)
		}

		logging.LogForComponent("extensionFactory").Infof("Registered builtins from extension %s", builtinConf.Extension)
	}

	return nil
}

func (f *defaultFactory) MakeDatastore(ctx context.Context, dsType string) (data.Datastore, error) {
	if !f.configured {
		return nil, errors.New("extension factory not configured")
	}

	dsExt, ok := f.datastores[dsType]
	if !ok {
		return nil, errors.Errorf("no datastore for type %q found", dsType)
	}

	return (*dsExt).Datastore(), nil
}

func (f *defaultFactory) loadExtensionsFromDir(dirPath string) error {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	for _, entry := range dirEntries {
		fullPath := fmt.Sprintf("%s%s", dirPath, entry.Name())

		if entry.IsDir() {
			if childErr := f.loadExtensionsFromDir(fullPath); childErr != nil {
				return childErr
			}
			continue
		}

		if isPluginFile(fullPath) {
			p, perr := plugin.Open(fullPath)
			if perr != nil {
				return errors.Wrapf(perr, "unable to open plugin at %s", fullPath)
			}

			rawExt, lerr := p.Lookup(constants.ExtensionInitSymbolName)
			if lerr != nil {
				return errors.Wrap(lerr, "unable to find extension init symbol")
			}

			funcNewExt, ok := rawExt.(func() extensions.Extension)
			if !ok {
				return errors.Errorf("extension loaded from %s does not properly implement NewExtension functions", fullPath)
			}
			ext := funcNewExt()

			// add as Builtin Extension
			if b, bok := ext.(extensions.ExtensionBuiltin); bok {
				logging.LogForComponent("extensionFactory").Infof("Adding Extension %q", b.Name())
				f.builtins[b.Name()] = &b
			}

			// add as Datastore Extension
			if d, dok := ext.(extensions.ExtensionDatastore); dok {
				logging.LogForComponent("extensionFactory").Infof("Adding Datastore %q", d.Name())
				key := fmt.Sprintf("%s.%s", d.Name(), d.Type())
				f.datastores[key] = &d
			}
		}
	}
	return nil
}

func isPluginFile(path string) bool {
	return strings.HasSuffix(path, ".so")
}

func normalizeDirPath(path string) string {
	if !strings.HasSuffix(path, "/") {
		return fmt.Sprintf("%s/", path)
	}
	return path
}
