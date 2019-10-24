package watcher

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/watcher"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
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
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case event, ok := <-fileWatcher.Events:
				if !ok {
					log.Warnln("Received invalid event while watching files.")
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
						log.Warnf("Unbable to get information about file %q", event.Name)
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
		change = watcher.CHANGE_REGO
		log.Println("FileConfigWatcher: update observers due to REGO change: ", event)
	case ".yml":
		change = watcher.CHANGE_CONF
		log.Println("FileConfigWatcher: update observers due to CONF change: ", event)
	case ".yaml":
		change = watcher.CHANGE_CONF
		log.Println("FileConfigWatcher: update observers due to CONF change: ", event)
	default:
		change = watcher.CHANGE_UNKNOWN
		log.Println("FileConfigWatcher: update observers due to UNKNOWN change: ", event)
	}
	return change
}

func closeWatcherOnSIGTERM(fileWatcher *fsnotify.Watcher) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	log.Infoln("Closing FileWatcher...")
	if fileWatcher != nil {
		if err := fileWatcher.Close(); err != nil {
			log.Fatalln(err.Error())
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
				log.Infof("Start watching path %q", path)
				err = fileWatcher.Add(path)
				if err != nil {
					log.Fatal(err)
				}
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}
}

// See pkg.watcher.ConfigWatcher
func (w *fileConfigWatcher) Watch(callback func(watcher.ChangeType, *configs.ExternalConfig, error)) {
	w.observers = append(w.observers, callback)
	loaded, err := w.loader.Load()
	callback(watcher.CHANGE_ALL, loaded, err)
}
