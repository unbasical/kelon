package watcher

import (
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/watcher"
)

// Implements pkg.watcher.ConfigWatcher by loading local files.
func NewMock() watcher.ConfigWatcher {
	return &mockConfigWatcher{}
}

type mockConfigWatcher struct{}

// See pkg.watcher.ConfigWatcher
func (w *mockConfigWatcher) Watch(callback func(watcher.ChangeType, *configs.ExternalConfig, error)) {
}
