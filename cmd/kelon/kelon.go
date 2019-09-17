package main

import (
	"fmt"
	"github.com/Foundato/kelon/configs"
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
)

type AppConfig struct {
	debug bool
}

func main() {
	app.HelpFlag.Short('h')
	app.Version("0.1.0")

	appConf := AppConfig{}

	// Parse args
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case start.FullCommand():
		appConf.debug = false
	case debug.FullCommand():
		appConf.debug = true
	}

	// Run app
	println("Kelon starting...")
	runApp(appConf)
}

func runApp(appConf AppConfig) {
	config, err := configs.FileConfigLoader{
		DatastoreConfigPath: *datastorePath,
		ApiConfigPath:       *apiPath,
	}.Load()

	if appConf.debug {
		println("Started in debug-mode")
	}
	if err != nil {
		log.Fatalln("Unable to parse configuration: ", err.Error())
	}

	fmt.Printf("%+v\n", config)
}
