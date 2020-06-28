package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Foundato/kelon/pkg/constants"

	"github.com/Foundato/kelon/pkg/telemetry"

	apiInt "github.com/Foundato/kelon/internal/pkg/api"
	opaInt "github.com/Foundato/kelon/internal/pkg/opa"
	requestInt "github.com/Foundato/kelon/internal/pkg/request"
	translateInt "github.com/Foundato/kelon/internal/pkg/translate"
	watcherInt "github.com/Foundato/kelon/internal/pkg/watcher"

	"github.com/Foundato/kelon/common"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/api/envoy"
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

//nolint:gochecknoglobals
var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")

	// Commands
	run = app.Command("run", "Run kelon in production mode.")

	// Flags
	datastorePath         = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
	apiPath               = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
	configWatcherPath     = app.Flag("config-watcher-path", "Path where the config watcher should listen for changes.").Envar("CONFIG_WATCHER_PATH").ExistingDir()
	opaPath               = app.Flag("opa-conf", "Path to the OPA configuration yaml.").Short('o').Default("./opa.yml").Envar("OPA_CONF").ExistingFile()
	regoDir               = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Short('r').Envar("REGO_DIR").ExistingDir()
	pathPrefix            = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-API.").Default("/v1").Envar("PATH_PREFIX").String()
	port                  = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Uint32()
	preprocessRegos       = app.Flag("preprocess-policies", "Preprocess incoming policies for internal use-case (EXPERIMENTAL FEATURE! DO NOT USE!).").Default("false").Envar("PREPROCESS_POLICIES").Bool()
	logLevel              = app.Flag("log-level", "Log-Level for Kelon. Must be one of [DEBUG, INFO, WARN, ERROR]").Default("INFO").Envar("LOG_LEVEL").Enum("DEBUG", "INFO", "WARN", "ERROR", "debug", "info", "warn", "error")
	respondWithStatusCode = app.Flag("respond-with-status-code", "Communicate Decision via status code 200 (ALLOW) or 403 (DENY).").Default("false").Envar("RESPOND_WITH_STATUS_CODE").Bool()

	// Configs for envoy external auth
	envoyPort       = app.Flag("envoy-port", "Also start Envoy GRPC-Proxy on specified port so integrate kelon with Istio.").Envar("ENVOY_PORT").Uint32()
	envoyDryRun     = app.Flag("envoy-dry-run", "Enable/Disable the dry run feature of the envoy-proxy.").Default("false").Envar("ENVOY_DRY_RUN").Bool()
	envoyReflection = app.Flag("envoy-reflection", "Enable/Disable the reflection feature of the envoy-proxy.").Default("true").Envar("ENVOY_REFLECTION").Bool()

	// Configs for telemetry
	telemetryService                = app.Flag("telemetry-service", "Service that is used for telemetry [Prometheus, ApplicationInsights]").Envar("TELEMETRY_SERVICE").Enum("Prometheus", "prometheus", "ApplicationInsights", "applicationinsights")
	instrumentationKey              = app.Flag("instrumentation-key", "The ApplicationInsights-InstrumentationKey that is used to connect to the API.").Envar("INSTRUMENTATION_KEY").String()
	appInsightsServiceName          = app.Flag("application-insights-service-name", "The name which will be displayed for kelon inside application insights.").Default("Kelon").Envar("APPLICATION_INSIGHTS_SERVICE_NAME").String()
	appInsightsMaxBatchSize         = app.Flag("application-insights-max-batch-size", "Configure how many items can be sent in one call to the data collector.").Default("8192").Envar("APPLICATION_INSIGHTS_MAX_BATCH_SIZE").Int()
	appInsightsMaxBatchInterval     = app.Flag("application-insights-max-batch-interval-seconds", "Configure the maximum delay before sending queued telemetry.").Default("2").Envar("APPLICATION_INSIGHTS_MAX_BATCH_INTERVAL_SECONDS").Int()
	appInsightsLogLevels            = app.Flag("application-insights-log-levels", "Configure log levels which are sent. Allowed values are [fatal, panic, error, warn, info, debug, trace]").Default("fatal,panic,error,warn").Envar("APPLICATION_INSIGHTS_LOG_LEVELS").String()
	appInsightsStatsIntervalSeconds = app.Flag("application-insights-stats-interval-seconds", "Interval in seconds in which system stats are measured and sent.").Default("5").Envar("APPLICATION_INSIGHTS_STATS_INTERVAL_SECONDS").Int()

	proxy             api.ClientProxy       = nil
	envoyProxy        api.ClientProxy       = nil
	configWatcher     watcher.ConfigWatcher = nil
	telemetryProvider telemetry.Provider    = nil
)

func main() {
	app.HelpFlag.Short('h')
	app.Version(common.Version)

	// Process args and initialize logger
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case run.FullCommand():
		log.SetOutput(os.Stdout)
		log.Infof("Kelon starting with log level %q...", *logLevel)
		switch strings.ToUpper(*logLevel) {
		case "INFO":
			log.SetLevel(log.InfoLevel)
		case "DEBUG":
			log.SetLevel(log.DebugLevel)
		case "WARN":
			log.SetLevel(log.WarnLevel)
		case "ERROR":
			log.SetLevel(log.ErrorLevel)
		}

		// Init config loader
		configLoader := configs.FileConfigLoader{
			DatastoreConfigPath: *datastorePath,
			APIConfigPath:       *apiPath,
		}
		// Start app after config is present
		makeConfigWatcher(configLoader, configWatcherPath)
		configWatcher.Watch(onConfigLoaded)
		stopOnSIGTERM()
	}
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

		// Build configs
		config.API = loadedConf.API
		config.Data = loadedConf.Data
		config.TelemetryProvider = makeTelemetryProvider()
		telemetryProvider = config.TelemetryProvider // Stopped gracefully later on
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
	}
}

func makeTelemetryProvider() telemetry.Provider {
	var provider telemetry.Provider
	if telemetryService != nil {
		switch strings.ToLower(*telemetryService) {
		case constants.PrometheusTelemetry:
			provider = &telemetry.Prometheus{}
		case constants.ApplicationInsightsTelemetry:
			provider = &telemetry.ApplicationInsights{
				AppInsightsInstrumentationKey: *instrumentationKey,
				ServiceName:                   *appInsightsServiceName,
				MaxBatchSize:                  *appInsightsMaxBatchSize,
				MaxBatchIntervalSeconds:       *appInsightsMaxBatchInterval,
				LogLevels:                     *appInsightsLogLevels,
				StatsIntervalSeconds:          *appInsightsStatsIntervalSeconds,
			}
		}

		if provider != nil {
			if err := provider.Configure(); err != nil {
				log.Fatalf("Error during configuration of TelemetryProvider %q: %s", *telemetryService, err.Error())
			}
		}
	}
	return provider
}

func makeConfigWatcher(configLoader configs.FileConfigLoader, configWatcherPath *string) {
	if regoDir == nil || *regoDir == "" {
		configWatcher = watcherInt.NewSimple(configLoader)
	} else {
		// Set configWatcherPath to rego path by default
		if configWatcherPath == nil || *configWatcherPath == "" {
			configWatcherPath = regoDir
		}
		configWatcher = watcherInt.NewFileWatcher(configLoader, *configWatcherPath)
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
		log.Panic("Cannot start envoyProxy proxy and rest proxy on same port!")
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

func makeServerConfig(compiler opa.PolicyCompiler, parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			RespondWithStatusCode: *respondWithStatusCode,
			Prefix:                pathPrefix,
			OpaConfigPath:         opaPath,
			RegoDir:               regoDir,
			ConfigWatcher:         &configWatcher,
			PathProcessor:         &parser,
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
	// Stop telemetry provider if present
	// This is done blocking to ensure all telemetries are sent!
	if telemetryProvider != nil {
		telemetryProvider.Shutdown()
	}

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
