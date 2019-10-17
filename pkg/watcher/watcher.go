// Package watcher contains components that are used for configuration reloading of kelon.
package watcher

import "github.com/Foundato/kelon/configs"

// ConfigWatcher is the interface that manages configuration reloading.
//
// Therefore a callback procedure is provided, which is always called if any configuration changes.
type ConfigWatcher interface {

	// Watches for configuration changes and calls the passed callback procedure every
	// time the config changes.
	Watch(callback func(*configs.ExternalConfig, error))
}
