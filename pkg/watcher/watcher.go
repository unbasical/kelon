// Package watcher contains components that are used for configuration reloading of kelon.
package watcher

import "github.com/unbasical/kelon/configs"

// ConfigWatcher is the interface that manages configuration reloading.
//
// Therefore, a callback procedure is provided, which is always called if any configuration changes.
type ConfigWatcher interface {

	// Watch will monitor for configuration changes and calls the passed callback procedure every
	// time the config changes.
	Watch(callback func(ChangeType, *configs.ExternalConfig, error))
}

// ChangeType represent the type of changes that can occur during Watch()
type ChangeType int

const (
	// ChangeAll Passed to Watch() on initial load
	ChangeAll ChangeType = 0
	// ChangeRego Passed to Watch() if any file with ending '.rego' changed
	ChangeRego ChangeType = 1
	// ChangeConf Passed to Watch() if any file with ending .yml or .yaml changed
	ChangeConf ChangeType = 2
	// ChangeUnknown Passed to Watch() if any file with unknown file ending changed
	ChangeUnknown ChangeType = 3
)
