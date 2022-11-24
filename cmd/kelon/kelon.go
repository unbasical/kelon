package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/common"
	"github.com/unbasical/kelon/configs"
	apiInt "github.com/unbasical/kelon/internal/pkg/api"
	"github.com/unbasical/kelon/internal/pkg/api/envoy"
	"github.com/unbasical/kelon/internal/pkg/data"
	opaInt "github.com/unbasical/kelon/internal/pkg/opa"
	requestInt "github.com/unbasical/kelon/internal/pkg/request"
	translateInt "github.com/unbasical/kelon/internal/pkg/translate"
	"github.com/unbasical/kelon/internal/pkg/util"
	watcherInt "github.com/unbasical/kelon/internal/pkg/watcher"
	"github.com/unbasical/kelon/pkg/api"
	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/opa"
	"github.com/unbasical/kelon/pkg/request"
	"github.com/unbasical/kelon/pkg/telemetry"
	"github.com/unbasical/kelon/pkg/translate"
	"github.com/unbasical/kelon/pkg/watcher"
	"gopkg.in/alecthomas/kingpin.v2"
)

//nolint:gochecknoglobals,gocritic
var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")

	// Commands
	run      = app.Command("run", "Run kelon in production mode.")
	validate = app.Command("validate", "Run kelon in validate mode: validate policies by printing resulting datastore queries")

	// Config paths
	datastorePath     = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
	apiPath           = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
	configWatcherPath = app.Flag("config-watcher-path", "Path where the config watcher should listen for changes.").Envar("CONFIG_WATCHER_PATH").ExistingDir()
	opaPath           = app.Flag("opa-conf", "Path to the OPA configuration yaml.").Short('o').Default("./opa.yml").Envar("OPA_CONF").ExistingFile()
	regoDir           = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Short('r').Envar("REGO_DIR").ExistingDir()

	// Additional config
	pathPrefix            = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-API.").Default("/v1").Envar("PATH_PREFIX").String()
	port                  = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Uint32()
	preprocessRegos       = app.Flag("preprocess-policies", "Preprocess incoming policies for internal use-case (EXPERIMENTAL FEATURE! DO NOT USE!).").Default("false").Envar("PREPROCESS_POLICIES").Bool()
	respondWithStatusCode = app.Flag("respond-with-status-code", "Communicate Decision via status code 200 (ALLOW) or 403 (DENY).").Default("false").Envar("RESPOND_WITH_STATUS_CODE").Bool()
	astSkipUnknown        = app.Flag("ast-skip-unknown", "Skip unknown parts in the AST and only log as warning.").Default("false").Envar("AST_SKIP_UNKNOWN").Bool()

	// Logging
	logLevel               = app.Flag("log-level", "Log-Level for Kelon. Must be one of [DEBUG, INFO, WARN, ERROR]").Default("INFO").Envar("LOG_LEVEL").Enum("DEBUG", "INFO", "WARN", "ERROR", "debug", "info", "warn", "error")
	logFormat              = app.Flag("log-format", "Log-Format for Kelon. Must be one of [TEXT, JSON]").Default("TEXT").Envar("LOG_FORMAT").Enum("TEXT", "JSON")
	accessDecisionLogLevel = app.Flag("access-decision-log-level", "Access decision Log-Level for Kelon. Must be one of [ALL, ALLOW, DENY, NONE]").Default("ALL").Envar("ACCESS_DECISION_LOG_LEVEL").Enum("ALL", "ALLOW", "DENY", "NONE", "all", "allow", "deny", "none")

	// Configs for envoy external auth
	envoyPort       = app.Flag("envoy-port", "Also start Envoy GRPC-Proxy on specified port so integrate kelon with Istio.").Envar("ENVOY_PORT").Uint32()
	envoyDryRun     = app.Flag("envoy-dry-run", "Enable/Disable the dry run feature of the envoy-proxy.").Default("false").Envar("ENVOY_DRY_RUN").Bool()
	envoyReflection = app.Flag("envoy-reflection", "Enable/Disable the reflection feature of the envoy-proxy.").Default("true").Envar("ENVOY_REFLECTION").Bool()

	// Configs for telemetry
	metricService        = app.Flag("metric-service", "Service that is used for metrics [Prometheus|OTLP]").Envar("METRIC_SERVICE").Enum("Prometheus", "prometheus", "OTLP", "otlp")
	metricExportProtocol = app.Flag("metric-export-protocol", "If metrics are exported with OTLP, select the protocol to use [http|grpc]").Default("http").Envar("METRIC_EXPORT_PROTOCOL").Enum("http", "grpc")
	metricExportEndpoint = app.Flag("metric-export-endpoint", "If metrics are exported with OTLP, this is the endpoint they will be exported to").Envar("METRIC_EXPORT_ENDPOINT").String()
	traceService         = app.Flag("trace-service", "Service that is used for tracing [OTLP]").Envar("TRACE_SERVICE").Enum("OTLP", "otlp")
	traceExportProtocol  = app.Flag("trace-export-protocol", "If traces are exported with OTLP, select the protocol to use [http|grpc]").Default("http").Envar("TRACE_EXPORT_PROTOCOL").Enum("http", "grpc")
	traceExportEndpoint  = app.Flag("trace-export-endpoint", "If traces are exported with OTLP, this is the endpoint they will be exported to").Envar("TRACE_EXPORT_ENDPOINT").String()

	// Configs for validate mode
	inputBody           = app.Flag("input-body", "Input Body to use in dry run mode").Envar("DRY_INPUT_BODY").String()
	queryOutputFilename = app.Flag("query-output", "File to write the Query to (JSON). If not set, write to stdout using logging format").Envar("QUERY_OUTPUT_FILE").String()

	// Global shared variables
	validateMode                              = false
	proxy           api.ClientProxy           = nil
	envoyProxy      api.ClientProxy           = nil
	configWatcher   watcher.ConfigWatcher     = nil
	metricsProvider telemetry.MetricsProvider = nil
	traceProvider   telemetry.TraceProvider   = nil
)

func main() {
	app.HelpFlag.Short('h')
	app.Version(common.Version)

	// Process args and initialize logger
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case validate.FullCommand():
		log.SetOutput(os.Stdout)

		validateMode = true

		// Set log format
		setLogFormat()

		// Set log level
		setLogLevel()

		// Start app in dry run mode
		makeConfigWatcher(configs.FileConfigLoader{}, configWatcherPath)
		dryRunRequest()

	case run.FullCommand():
		log.SetOutput(os.Stdout)

		// Set log format
		setLogFormat()

		// Set log level
		setLogLevel()

		// Init config loader
		configLoader := configs.FileConfigLoader{
			DatastoreConfigPath: *datastorePath,
			APIConfigPath:       *apiPath,
		}

		// Start app after config is present
		makeConfigWatcher(configLoader, configWatcherPath)
		configWatcher.Watch(onConfigLoaded)
		stopOnSIGTERM()

	default:
		logging.LogForComponent("main").Fatal("Started Kelon with a unknown command!")
	}
}

func setLogFormat() {
	switch *logFormat {
	case "JSON":
		log.SetFormatter(util.UTCFormatter{Formatter: &log.JSONFormatter{}})
	default:
		log.SetFormatter(util.UTCFormatter{Formatter: &log.TextFormatter{FullTimestamp: true}})
	}
}

func setLogLevel() {
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
	logging.LogForComponent("main").Infof("Kelon starting with log level %q...", *logLevel)
}

func onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		logging.LogForComponent("main").Fatalln("Unable to parse configuration: ", err.Error())
	}

	ctx := context.Background()

	if change == watcher.ChangeAll {
		// Configure application
		var (
			config     = new(configs.AppConfig)
			compiler   = opaInt.NewPolicyCompiler()
			parser     = requestInt.NewURLProcessor()
			mapper     = requestInt.NewPathMapper()
			translator = translateInt.NewAstTranslator()
		)

		// Build config
		config.API = loadedConf.API
		config.Data = loadedConf.Data
		config.MetricsProvider = makeTelemetryMetricsProvider(ctx)
		metricsProvider = config.MetricsProvider // Stopped gracefully later on
		config.TraceProvider = makeTelemetryTraceProvider(ctx)
		traceProvider = config.TraceProvider // Stopped gracefully later on
		serverConf := makeServerConfig(compiler, parser, mapper, translator, loadedConf)

		if *preprocessRegos {
			*regoDir = util.PrepocessPoliciesInDir(config, *regoDir)
		}

		// Start rest proxy
		startNewRestProxy(ctx, config, &serverConf)

		// Start envoyProxy proxy in addition to rest proxy as soon as a port was specified!
		if envoyPort != nil && *envoyPort != 0 {
			startNewEnvoyProxy(ctx, config, &serverConf)
		}
	}
}

func dryRunRequest() {
	confLoader := configs.FileConfigLoader{DatastoreConfigPath: *datastorePath, APIConfigPath: *apiPath}
	loadedConf, err := confLoader.Load()
	if err != nil {
		logging.LogForComponent("main").Fatalln("Unable to parse configuration: ", err.Error())
	}

	// Create logging file
	if queryOutputFilename != nil && *queryOutputFilename != "" {
		f, fileErr := os.Create(*queryOutputFilename)
		if fileErr != nil {
			logging.LogForComponent("main").Fatalln("Unable to parse crate file: ", fileErr.Error())
		}

		defer f.Close()
		loadedConf.Data.OutputFile = f
	}

	ctx := context.Background()
	// Configure application
	var (
		config     = new(configs.AppConfig)
		compiler   = opaInt.NewPolicyCompiler()
		parser     = requestInt.NewURLProcessor()
		mapper     = requestInt.NewPathMapper()
		translator = translateInt.NewAstTranslator()
	)

	// Build config
	config.API = loadedConf.API
	config.Data = loadedConf.Data
	config.MetricsProvider = telemetry.NewNoopMetricProvider()
	metricsProvider = config.MetricsProvider // Stopped gracefully later on
	config.TraceProvider = telemetry.NewNoopTraceProvider()
	traceProvider = config.TraceProvider // Stopped gracefully later on
	serverConf := makeServerConfig(compiler, parser, mapper, translator, loadedConf)

	if *preprocessRegos {
		*regoDir = util.PrepocessPoliciesInDir(config, *regoDir)
	}

	err = (*serverConf.Compiler).Configure(config, &serverConf.PolicyCompilerConfig)
	if err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}

	body, err := parseBodyFromString(*inputBody)
	if err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}

	// Execute Request
	if _, err := compiler.Execute(ctx, body); err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}
}

func makeTelemetryMetricsProvider(ctx context.Context) telemetry.MetricsProvider {
	if metricService != nil && *metricService != "" {
		provider, err := telemetry.NewMetricsProvider(ctx, constants.TelemetryServiceName, *metricService, *metricExportProtocol, *metricExportEndpoint)
		if err != nil {
			logging.LogForComponent("main").Fatalf("Error during creation of MetricsProvider %q: %s", *metricService, err)
		}

		if err := provider.Configure(ctx); err != nil {
			logging.LogForComponent("main").Fatalf("Error during configuration of MetricsProvider %q: %s", *metricService, err.Error())
		}

		return provider
	}
	return telemetry.NewNoopMetricProvider()
}

func makeTelemetryTraceProvider(ctx context.Context) telemetry.TraceProvider {
	if traceService != nil && *traceService != "" {
		provider, err := telemetry.NewTraceProvider(ctx, constants.TelemetryServiceName, *traceExportProtocol, *traceExportEndpoint)
		if err != nil {
			logging.LogForComponent("main").Fatalf("Error during creation of TraceProvider %q: %s", *traceService, err)
		}

		if err := provider.Configure(ctx); err != nil {
			logging.LogForComponent("main").Fatalf("Error during configuration of TraceProvider %q: %s", *traceService, err.Error())
		}

		return provider
	}
	return telemetry.NewNoopTraceProvider()
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

func startNewRestProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	// Create Rest proxy and start
	proxy = apiInt.NewRestProxy(*pathPrefix, int32(*port))
	if err := proxy.Configure(ctx, appConfig, serverConf); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
	// Start proxy
	if err := proxy.Start(); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
}

func startNewEnvoyProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	if *envoyPort == *port {
		logging.LogForComponent("main").Panic("Cannot start envoyProxy proxy and rest proxy on same port!")
	}

	// Create Rest proxy and start
	envoyProxy = envoy.NewEnvoyProxy(envoy.Config{
		Port:                   *envoyPort,
		DryRun:                 *envoyDryRun,
		EnableReflection:       *envoyReflection,
		AccessDecisionLogLevel: *accessDecisionLogLevel,
	})
	if err := envoyProxy.Configure(ctx, appConfig, serverConf); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
	// Start proxy
	if err := envoyProxy.Start(); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
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
				Datastores:  data.MakeDatastores(loadedConf.Data, validateMode),
				SkipUnknown: *astSkipUnknown,
			},
			AccessDecisionLogLevel: strings.ToUpper(*accessDecisionLogLevel),
		},
	}
	return serverConf
}

func parseBodyFromString(input string) (map[string]interface{}, error) {
	in := []byte(input)
	var raw map[string]interface{}

	err := json.Unmarshal(in, &raw)
	return raw, err
}

func stopOnSIGTERM() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	logging.LogForComponent("main").Infoln("Caught SIGTERM...")

	// Stop metrics provider
	// This is done blocking to ensure all metrics are sent!
	metricsProvider.Shutdown(context.Background())

	// Stop trace provider
	// This is done blocking to ensure all traces are sent!
	traceProvider.Shutdown(context.Background())

	// Stop envoyProxy proxy if started
	if envoyProxy != nil {
		if err := envoyProxy.Stop(time.Second * 10); err != nil {
			logging.LogForComponent("main").Warnln(err.Error())
		}
	}

	// Stop rest proxy if started
	if proxy != nil {
		if err := proxy.Stop(time.Second * 10); err != nil {
			logging.LogForComponent("main").Warnln(err.Error())
		}
	}
	// Give components enough time for graceful shutdown
	// This terminates earlier, because rest-proxy prints FATAL if http-server is closed
	time.Sleep(5 * time.Second)
}
