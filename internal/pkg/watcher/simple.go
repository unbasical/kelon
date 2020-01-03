package watcher

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/watcher"
)

// Implements pkg.watcher.ConfigWatcher by loading local files.
func NewSimple(loader configs.ConfigLoader) watcher.ConfigWatcher {
	newWatcher := simpleConfigWatcher{
		loader: loader,
	}
	return &newWatcher
}

type simpleConfigWatcher struct {
	loader configs.ConfigLoader
}

// See pkg.watcher.ConfigWatcher
func (w *simpleConfigWatcher) Watch(callback func(watcher.ChangeType, *configs.ExternalConfig, error)) {
	loaded, err := w.loader.Load()
	callback(watcher.ChangeAll, loaded, err)
}
