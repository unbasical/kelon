package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	apiInt "github.com/Foundato/kelon/internal/pkg/api"
	opaInt "github.com/Foundato/kelon/internal/pkg/opa"
	requestInt "github.com/Foundato/kelon/internal/pkg/request"
	translateInt "github.com/Foundato/kelon/internal/pkg/translate"
	watcherInt "github.com/Foundato/kelon/internal/pkg/watcher"

	"github.com/Foundato/kelon/common"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/Foundato/kelon/pkg/opa"
	"github.com/Foundato/kelon/pkg/request"
	"github.com/Foundato/kelon/pkg/translate"
	"github.com/Foundato/kelon/pkg/watcher"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	//nolint:gochecknoglobals
	app = kingpin.New("kelon", "Kelon policy enforcer.")
	//nolint:gochecknoglobals
	opaPath = app.Flag("opa-conf", "Path to the OPA configuration yaml.").Short('o').Default("./opa.yml").Envar("OPA_CONF").ExistingFile()
	//nolint:gochecknoglobals
	regoDir = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Default("./").Short('r').Envar("REGO_DIR").ExistingDir()
	//nolint:gochecknoglobals
	pathPrefix = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-API.").Default("/v1").Envar("PATH_PREFIX").String()
	//nolint:gochecknoglobals
	port = app.Flag("port", "port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Int32()
	//nolint:gochecknoglobals
	proxy api.ClientProxy = nil
	//nolint:gochecknoglobals
	configWatcher watcher.ConfigWatcher = nil
)

func main() {
	// Configure kingpin
	var (
		// Commands
		start = app.Command("start", "Start kelon in production mode.")
		debug = app.Command("debug", "Enable debug mode.")
		// Flags
		datastorePath     = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
		apiPath           = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
		configWatcherPath = app.Flag("config-watcher-path", "Path where the config watcher should listen for changes.").Default("./").Envar("CONFIG_WATCHER_PATH").ExistingDir()
	)

	app.HelpFlag.Short('h')
	app.Version(common.Version)
	// Process args
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case start.FullCommand():
		log.SetOutput(os.Stdout)
		log.SetLevel(log.InfoLevel)
		log.Infoln("Kelon starting...")

	case debug.FullCommand():
		log.SetOutput(os.Stdout)
		log.SetLevel(log.DebugLevel)
		log.Infoln("Kelon starting in debug-mode...")
	}

	// Init config loader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: *datastorePath,
		APIConfigPath:       *apiPath,
	}
	// Start app after config is present
	configWatcher = watcherInt.NewFileWatcher(configLoader, *configWatcherPath)
	configWatcher.Watch(onConfigLoaded)
	stopOnSIGTERM()
}

func onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		log.Fatalln("Unable to parse configuration: ", err.Error())
	}

	switch change {
	// First update
	case watcher.CHANGE_ALL:
		startNewRestProxy(loadedConf)
	}
}

func startNewRestProxy(loadedConf *configs.ExternalConfig) {
	// Configure application
	var (
		config     = new(configs.AppConfig)
		compiler   = opaInt.NewPolicyCompiler()
		parser     = requestInt.NewURLProcessor()
		mapper     = requestInt.NewPathMapper()
		translator = translateInt.NewAstTranslator()
	)
	// Build app config
	config.API = loadedConf.API
	config.Data = loadedConf.Data
	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        pathPrefix,
			OpaConfigPath: opaPath,
			RegoDir:       regoDir,
			ConfigWatcher: &configWatcher,
			PathProcessor: &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores: data.MakeDatastores(loadedConf.Data),
			},
		},
	}
	// Create Rest proxy and start
	proxy = apiInt.NewRestProxy(*pathPrefix, *port)
	if err := proxy.Configure(config, &serverConf); err != nil {
		log.Fatalln(err.Error())
	}
	// Start proxy
	if err := proxy.Start(); err != nil {
		log.Fatalln(err.Error())
	}
}

func stopOnSIGTERM() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	log.Infoln("Caught SIGTERM...")
	if proxy != nil {
		if err := proxy.Stop(time.Second * 10); err != nil {
			log.Fatalln(err.Error())
		}
	}
}
