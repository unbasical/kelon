package main

import (
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/api"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/Foundato/kelon/internal/pkg/opa"
	"github.com/Foundato/kelon/internal/pkg/request"
	"github.com/Foundato/kelon/internal/pkg/translate"
	"github.com/Foundato/kelon/internal/pkg/watcher"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Configure kingpin
var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")
	// Commands
	start = app.Command("start", "Start kelon in production mode.")
	debug = app.Command("debug", "Enable debug mode.")
	// Flags
	datastorePath = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
	apiPath       = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
	opaPath       = app.Flag("opa-conf", "Path to the OPA configuration yaml.").Short('o').Default("./opa.yml").Envar("OPA_CONF").ExistingFile()
	regoDir       = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Default("./").Short('r').Envar("REGO_DIR").ExistingDir()
	pathPrefix    = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-Api.").Default("/v1").Envar("PATH_PREFIX").String()
	port          = app.Flag("port", "port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Int32()
)

// Configure application
var (
	config                 = new(configs.AppConfig)
	proxy  api.ClientProxy = nil

	compiler   = opa.NewPolicyCompiler()
	parser     = request.NewUrlProcessor()
	mapper     = request.NewPathMapper()
	translator = translate.NewAstTranslator()
)

func main() {
	app.HelpFlag.Short('h')
	app.Version("0.1.0")

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
		ApiConfigPath:       *apiPath,
	}
	// Start app after config is present
	watcher.DefaultConfigWatcher{Loader: configLoader}.Watch(onConfigLoaded)

	stopOnSIGTERM()
}

func onConfigLoaded(loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		log.Fatalln("Unable to parse configuration: ", err.Error())
	}

	// Build app config
	config.Api = loadedConf.Api
	config.Data = loadedConf.Data

	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        pathPrefix,
			OpaConfigPath: opaPath,
			RegoDir:       regoDir,
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
	proxy = api.NewRestProxy(*pathPrefix, *port)
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
