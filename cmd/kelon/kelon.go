package main

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/api"
	"github.com/Foundato/kelon/internal/pkg/watcher"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"os"
)

var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")
	// Commands
	start = app.Command("start", "Start kelon in production mode.")
	debug = app.Command("debug", "Enable debug mode.")
	// Flags
	datastorePath = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
	apiPath       = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
	pathPrefix    = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-Api.").Default("/v1/data").Envar("PATH_PREFIX").String()
	port          = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Int32()
)

var config = new(configs.AppConfig)

func main() {
	app.HelpFlag.Short('h')
	app.Version("0.1.0")

	// Parse args
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case start.FullCommand():
		config.Debug = false
		log.Println("Kelon starting...")
	case debug.FullCommand():
		config.Debug = true
		log.Println("Kelon starting in debug-mode...")
	}

	// Init config loader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: *datastorePath,
		ApiConfigPath:       *apiPath,
	}
	// Start app after config is present
	watcher.DefaultConfigWatcher{Loader: configLoader}.Watch(onConfigLoaded)
}

func onConfigLoaded(loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		log.Fatalln("Unable to parse configuration: ", err.Error())
	}
	config.Api = loadedConf.Api
	config.Data = loadedConf.Data

	// Create Server and Route Handlers
	log.Printf("Serving OPA's Data-Api at: http://localhost:%d%s\n", *port, *pathPrefix)
	restProxy := api.NewRestProxy(*pathPrefix, *port, config)
	restProxy.Start()
}
