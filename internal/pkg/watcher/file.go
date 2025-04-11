package watcher

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/watcher"
)

// NewFileWatcher instantiates a new watcher.ConfigWatcher by loading local files.
func NewFileWatcher(loader configs.ConfigLoader, watchPath string) watcher.ConfigWatcher {
	newWatcher := fileConfigWatcher{
		loader:    loader,
		watchPath: watchPath,
	}

	newWatcher.watchForChanges()
	return &newWatcher
}

type fileConfigWatcher struct {
	loader    configs.ConfigLoader
	watchPath string
	observers []func(watcher.ChangeType, *configs.ExternalConfig, error)
}

func (w *fileConfigWatcher) watchForChanges() {
	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		logging.LogForComponent("fileConfigWatcher").Fatal(err)
	}

	go w.watchRoutine(fileWatcher)
	addWatchDirsRecursive(w, fileWatcher)
	go closeWatcherOnSIGTERM(fileWatcher)
}

// watchRoutine reacts to file change events and triggers observer
func (w *fileConfigWatcher) watchRoutine(fileWatcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-fileWatcher.Events:
			if !ok {
				logging.LogForComponent("fileConfigWatcher").Warnln("Received invalid event while watching files.")
				return
			}

			// Notify observers if a file was created, modified or deleted
			if isRelevantEvent(event) {
				change := extractChangeType(event)

				// Notify observers for changes
				loaded, err := w.loader.Load()
				for _, observer := range w.observers {
					observer(change, loaded, err)
				}
			}
		case err, ok := <-fileWatcher.Errors:
			if !ok {
				return
			}
			logging.LogForComponent("fileConfigWatcher").Warnf("fsnotify encountered an error: %s", err.Error())
		}
	}
}

// isRelevantEvent returns true if a file was created, modified or deleted
func isRelevantEvent(event fsnotify.Event) bool {
	// Gather information about event
	writeEvent := event.Op&fsnotify.Write == fsnotify.Write
	createEvent := event.Op&fsnotify.Create == fsnotify.Create
	removeEvent := event.Op&fsnotify.Remove == fsnotify.Remove
	isFile := false

	// Check if current modified file is File
	if !removeEvent {
		if fileInfo, err := os.Stat(event.Name); err == nil {
			isFile = !fileInfo.IsDir()
		} else {
			logging.LogForComponent("fileConfigWatcher").Warnf("Unable to get information about file %q", event.Name)
		}
	}

	return isFile && (createEvent || writeEvent || removeEvent)
}

// extractChangeType maps file system changes to internal change types in order to ease observer trigger filter
func extractChangeType(event fsnotify.Event) watcher.ChangeType {
	var change watcher.ChangeType
	extension := filepath.Ext(event.Name)
	switch extension {
	case ".rego":
		change = watcher.ChangeRego
		log.Println("FileConfigWatcher: update observers due to REGO change: ", event)
	case ".yml":
		change = watcher.ChangeConf
		log.Println("FileConfigWatcher: update observers due to CONF change: ", event)
	case ".yaml":
		change = watcher.ChangeConf
		log.Println("FileConfigWatcher: update observers due to CONF change: ", event)
	default:
		change = watcher.ChangeUnknown
		log.Println("FileConfigWatcher: update observers due to UNKNOWN change: ", event)
	}
	return change
}

// closeWatcherOnSIGTERM watches OS signals and channels and closes the watcher on termination signals
func closeWatcherOnSIGTERM(fileWatcher *fsnotify.Watcher) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	// Block until we receive our signal.
	<-interruptChan

	logging.LogForComponent("fileConfigWatcher").Infoln("Closing...")
	if fileWatcher != nil {
		if err := fileWatcher.Close(); err != nil {
			logging.LogForComponent("fileConfigWatcher").WithError(err).Fatalln("Unable to close file watcher")
		}
	}
}

// addWatchDirsRecursive adds directories recursively to the watch list
func addWatchDirsRecursive(configWatcher *fileConfigWatcher, fileWatcher *fsnotify.Watcher) {
	err := filepath.Walk(configWatcher.watchPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				logging.LogForComponent("fileConfigWatcher").Infof("Start watching path %q", path)
				err = fileWatcher.Add(path)
				if err != nil {
					logging.LogForComponent("fileConfigWatcher").WithError(err).Fatal("Unable to add path")
				}
			}
			return nil
		})
	if err != nil {
		logging.LogForComponent("fileConfigWatcher").WithError(err).Error("Error during filepath walk")
	}
}

// Watch - see watcher.ConfigWatcher
func (w *fileConfigWatcher) Watch(callback func(watcher.ChangeType, *configs.ExternalConfig, error)) {
	w.observers = append(w.observers, callback)
	loaded, err := w.loader.Load()
	callback(watcher.ChangeAll, loaded, err)
}
