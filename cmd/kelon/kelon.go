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
	"github.com/Foundato/kelon/internal/pkg/api/envoy"
	"github.com/Foundato/kelon/internal/pkg/api/istio"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/Foundato/kelon/internal/pkg/util"
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
	regoDir = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Default("./policies").Short('r').Envar("REGO_DIR").ExistingDir()
	//nolint:gochecknoglobals
	pathPrefix = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-API.").Default("/v1").Envar("PATH_PREFIX").String()
	//nolint:gochecknoglobals
	port = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Uint32()
	//nolint:gochecknoglobals
	envoyPort = app.Flag("envoy-port", "Also start Envoy GRPC-Proxy on specified port so integrate kelon with Istio.").Envar("ENVOY_PORT").Uint32()
	//nolint:gochecknoglobals
	envoyDryRun = app.Flag("envoy-dry-run", "Enable/Disable the dry run feature of the envoy-proxy.").Default("false").Envar("ENVOY_DRY_RUN").Bool()
	//nolint:gochecknoglobals
	envoyReflection = app.Flag("envoy-reflection", "Enable/Disable the reflection feature of the envoy-proxy.").Default("true").Envar("ENVOY_REFLECTION").Bool()
	//nolint:gochecknoglobals
	respondWithStatusCode = app.Flag("respond-with-status-code", "Communicate Decision via status code 200 (ALLOW) or 403 (DENY).").Default("false").Envar("RESPOND_WITH_STATUS_CODE").Bool()
	//nolint:gochecknoglobals
	istioPort = app.Flag("istio-port", "Also start Istio Mixer Out of Tree Adapter  on specified port so integrate kelon with Istio.").Envar("ISTIO_PORT").Uint32()
	//nolint:gochecknoglobals
	preprocessRegos = app.Flag("preprocess-policies", "Preprocess incoming policies for internal use-case (EXPERIMENTAL FEATURE! DO NOT USE!).").Default("false").Envar("PREPROCESS_POLICIES").Bool()

	//nolint:gochecknoglobals
	proxy api.ClientProxy = nil
	//nolint:gochecknoglobals
	envoyProxy api.ClientProxy = nil
	//nolint:gochecknoglobals
	istioProxy api.ClientProxy = nil
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
		configWatcherPath = app.Flag("config-watcher-path", "Path where the config watcher should listen for changes.").Default("./policies").Envar("CONFIG_WATCHER_PATH").ExistingDir()
	)

	app.HelpFlag.Short('h')
	app.Version(common.Version)

	// Process args and initialize logger
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
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

	if change == watcher.ChangeAll {
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
		serverConf := makeServerConfig(compiler, parser, mapper, translator, loadedConf)

		if *preprocessRegos {
			*regoDir = util.PrepocessPoliciesInDir(config, *regoDir)
		}

		// Start rest proxy
		startNewRestProxy(config, &serverConf)

		// Start envoyProxy proxy in addition to rest proxy as soon as a port was specified!
		if envoyPort != nil && *envoyPort != 0 {
			startNewEnvoyProxy(config, &serverConf)
		}

		// Start istio adapter in addition to rest proxy as soon as a port was specified!
		if istioPort != nil && *istioPort != 0 {
			startNewIstioAdapter(config, &serverConf)
		}
	}
}

func startNewRestProxy(appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	// Create Rest proxy and start
	proxy = apiInt.NewRestProxy(*pathPrefix, int32(*port))
	if err := proxy.Configure(appConfig, serverConf); err != nil {
		log.Fatalln(err.Error())
	}
	// Start proxy
	if err := proxy.Start(); err != nil {
		log.Fatalln(err.Error())
	}
}

func startNewEnvoyProxy(appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	if *envoyPort == *port {
		panic("Cannot start envoyProxy proxy and rest proxy on same port!")
	}
	if *envoyPort == *istioPort {
		panic("Cannot start envoyProxy proxy and istio adapter on same port!")
	}

	// Create Rest proxy and start
	envoyProxy = envoy.NewEnvoyProxy(envoy.EnvoyConfig{
		Port:             *envoyPort,
		DryRun:           *envoyDryRun,
		EnableReflection: *envoyReflection,
	})
	if err := envoyProxy.Configure(appConfig, serverConf); err != nil {
		log.Fatalln(err.Error())
	}
	// Start proxy
	if err := envoyProxy.Start(); err != nil {
		log.Fatalln(err.Error())
	}
}

func startNewIstioAdapter(appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	if *istioPort == *port {
		panic("Cannot start istio adapter and rest proxy on same port!")
	}
	if *envoyPort == *istioPort {
		panic("Cannot start envoyProxy proxy and istio adapter on same port!")
	}

	// Create Rest proxy and start
	istioProxy = istio.NewKelonIstioAdapter(*istioPort)
	if err := istioProxy.Configure(appConfig, serverConf); err != nil {
		log.Fatalln(err.Error())
	}
	// Start proxy
	if err := istioProxy.Start(); err != nil {
		log.Fatalln(err.Error())
	}
}

func makeServerConfig(compiler opa.PolicyCompiler, parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler:              &compiler,
		RespondWithStatusCode: *respondWithStatusCode,
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
	return serverConf
}

func stopOnSIGTERM() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	log.Infoln("Caught SIGTERM...")
	// Stop envoyProxy proxy if started
	if envoyProxy != nil {
		if err := envoyProxy.Stop(time.Second * 10); err != nil {
			log.Warnln(err.Error())
		}
	}

	// Stop rest proxy if started
	if proxy != nil {
		if err := proxy.Stop(time.Second * 10); err != nil {
			log.Warnln(err.Error())
		}
	}

	// Give components enough time for graceful shutdown
	// This terminates earlier, because rest-proxy prints FATAL if http-server is closed
	time.Sleep(5 * time.Second)
}
