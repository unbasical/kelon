package watcher

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/watcher"
)

// Implements pkg.watcher.ConfigWatcher by loading local files.
func NewFileWatcher(loader configs.ConfigLoader) watcher.ConfigWatcher {
	return &fileConfigWatcher{
		loader: loader,
	}
}

type fileConfigWatcher struct {
	loader configs.ConfigLoader
}

// See pkg.watcher.ConfigWatcher
func (w fileConfigWatcher) Watch(callback func(*configs.ExternalConfig, error)) {
	callback(w.loader.Load())
}
