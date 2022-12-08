package watcher

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/unbasical/kelon/pkg/constants/logging"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/watcher"
)

// Implements pkg.watcher.ConfigWatcher by loading local files.
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

	go func() {
		for {
			select {
			case event, ok := <-fileWatcher.Events:
				if !ok {
					logging.LogForComponent("fileConfigWatcher").Warnln("Received invalid event while watching files.")
					return
				}

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

				// Notify observers if a file was created, modified or deleted
				if isFile && (createEvent || writeEvent || removeEvent) {
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
				log.Println("error:", err)
			}
		}
	}()

	addWatchDirsRecursive(w, fileWatcher)
	go closeWatcherOnSIGTERM(fileWatcher)
}

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

// See pkg.watcher.ConfigWatcher
func (w *fileConfigWatcher) Watch(callback func(watcher.ChangeType, *configs.ExternalConfig, error)) {
	w.observers = append(w.observers, callback)
	loaded, err := w.loader.Load()
	callback(watcher.ChangeAll, loaded, err)
}
