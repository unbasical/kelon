package watcher

import (
	"github.com/Foundato/kelon/configs"
)

type DefaultConfigWatcher struct {
	Loader configs.ConfigLoader
}

func (w DefaultConfigWatcher) Watch(callback func(*configs.ExternalConfig, error)) {
	callback(w.Loader.Load())
}
