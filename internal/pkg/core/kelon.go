package core

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	apiInt "github.com/unbasical/kelon/internal/pkg/api"
	"github.com/unbasical/kelon/internal/pkg/api/envoy"
	"github.com/unbasical/kelon/internal/pkg/data"
	opaInt "github.com/unbasical/kelon/internal/pkg/opa"
	requestInt "github.com/unbasical/kelon/internal/pkg/request"
	translateInt "github.com/unbasical/kelon/internal/pkg/translate"
	watcherInt "github.com/unbasical/kelon/internal/pkg/watcher"
	"github.com/unbasical/kelon/pkg/api"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/opa"
	"github.com/unbasical/kelon/pkg/request"
	"github.com/unbasical/kelon/pkg/telemetry"
	"github.com/unbasical/kelon/pkg/translate"
	"github.com/unbasical/kelon/pkg/watcher"
)

type KelonConfiguration struct {
	// Config paths
	DatastorePath     *string
	APIPath           *string
	ConfigWatcherPath *string
	OpaPath           *string
	RegoDir           *string
	OperandDir        *string

	// Additional config
	PathPrefix     *string
	Port           *uint32
	AstSkipUnknown *bool

	// Logging
	AccessDecisionLogLevel *string

	// Configs for envoy external auth
	EnvoyPort       *uint32
	EnvoyDryRun     *bool
	EnvoyReflection *bool

	// Configs for telemetry
	MetricProvider           *string
	OtlpMetricExportProtocol *string
	OtlpMetricExportEndpoint *string
	TraceProvider            *string
	OtlpTraceExportProtocol  *string
	OtlpTraceExportEndpoint  *string
	OtlpServiceName          *string

	// Configs for validate mode
	Validate            bool
	InputBody           *string
	QueryOutputFilename *string
}

type Kelon struct {
	configured      bool
	config          *KelonConfiguration
	proxy           api.ClientProxy
	envoyProxy      api.ClientProxy
	configWatcher   watcher.ConfigWatcher
	metricsProvider telemetry.MetricsProvider
	traceProvider   telemetry.TraceProvider
}

func (k *Kelon) Configure(config *KelonConfiguration) {
	if k.configured {
		return
	}

	k.config = config
	k.configured = true
}

func (k *Kelon) Start() {
	if !k.configured {
		logging.LogForComponent("main").Fatalf("Kelon was not configured! Please call Configure()!")
	}

	// Init config loader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: *k.config.DatastorePath,
		APIConfigPath:       *k.config.APIPath,
	}

	// Start app after config is present
	k.makeConfigWatcher(configLoader, k.config.ConfigWatcherPath)
	k.configWatcher.Watch(k.onConfigLoaded)
	k.stopOnSIGTERM()
}

func (k *Kelon) StartValidate() {
	if !k.configured {
		logging.LogForComponent("main").Fatalf("Kelon was not configured! Please call Configure()!")
	}

	k.makeConfigWatcher(configs.FileConfigLoader{}, k.config.ConfigWatcherPath)

	confLoader := configs.FileConfigLoader{DatastoreConfigPath: *k.config.DatastorePath, APIConfigPath: *k.config.APIPath}
	loadedConf, err := confLoader.Load()
	if err != nil {
		logging.LogForComponent("main").Fatalln("Unable to parse configuration: ", err.Error())
	}

	// Create logging file
	if k.config.QueryOutputFilename != nil && *k.config.QueryOutputFilename != "" {
		f, fileErr := os.Create(*k.config.QueryOutputFilename)
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
	config.Data.CallOperandsDir = *k.config.OperandDir
	config.MetricsProvider = telemetry.NewNoopMetricProvider()
	k.metricsProvider = config.MetricsProvider // Stopped gracefully later on
	config.TraceProvider = telemetry.NewNoopTraceProvider()
	k.traceProvider = config.TraceProvider // Stopped gracefully later on
	serverConf := k.makeServerConfig(compiler, parser, mapper, translator, loadedConf)

	err = (*serverConf.Compiler).Configure(config, &serverConf.PolicyCompilerConfig)
	if err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}

	body, err := parseBodyFromString(*k.config.InputBody)
	if err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}

	// Execute Request
	d, err := compiler.Execute(ctx, body)
	if err != nil {
		logging.LogForComponent("main").Fatalf(err.Error())
	}

	var allowString string
	if d.Allow {
		allowString = "ALLOW"
	} else {
		allowString = "DENY"
	}

	logFields := log.Fields{
		logging.LabelPath:   d.Path,
		logging.LabelMethod: d.Method,
	}
	logging.LogAccessDecision(serverConf.AccessDecisionLogLevel, allowString, "main", logFields)
}

func (k *Kelon) onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error) {
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
		config.Data.CallOperandsDir = *k.config.OperandDir
		config.MetricsProvider = k.makeTelemetryMetricsProvider(ctx)
		k.metricsProvider = config.MetricsProvider // Stopped gracefully later on
		config.TraceProvider = k.makeTelemetryTraceProvider(ctx)
		k.traceProvider = config.TraceProvider // Stopped gracefully later on
		serverConf := k.makeServerConfig(compiler, parser, mapper, translator, loadedConf)

		// Start rest proxy
		k.startNewRestProxy(ctx, config, &serverConf)

		// Start envoyProxy proxy in addition to rest proxy as soon as a port was specified!
		if k.config.EnvoyPort != nil && *k.config.EnvoyPort != 0 {
			k.startNewEnvoyProxy(ctx, config, &serverConf)
		}
	}
}

func (k *Kelon) makeTelemetryMetricsProvider(ctx context.Context) telemetry.MetricsProvider {
	if k.config.MetricProvider != nil && *k.config.MetricProvider != "" {
		provider, err := telemetry.NewMetricsProvider(ctx, *k.config.OtlpServiceName, *k.config.MetricProvider, *k.config.OtlpMetricExportProtocol, *k.config.OtlpMetricExportEndpoint)
		if err != nil {
			logging.LogForComponent("main").Fatalf("Error during creation of MetricsProvider %q: %s", *k.config.MetricProvider, err)
		}

		if err := provider.Configure(ctx); err != nil {
			logging.LogForComponent("main").Fatalf("Error during configuration of MetricsProvider %q: %s", *k.config.MetricProvider, err.Error())
		}

		return provider
	}
	return telemetry.NewNoopMetricProvider()
}

func (k *Kelon) makeTelemetryTraceProvider(ctx context.Context) telemetry.TraceProvider {
	if k.config.TraceProvider != nil && *k.config.TraceProvider != "" {
		provider, err := telemetry.NewTraceProvider(ctx, *k.config.OtlpServiceName, *k.config.OtlpTraceExportProtocol, *k.config.OtlpTraceExportEndpoint)
		if err != nil {
			logging.LogForComponent("main").Fatalf("Error during creation of TraceProvider %q: %s", *k.config.TraceProvider, err)
		}

		if err := provider.Configure(ctx); err != nil {
			logging.LogForComponent("main").Fatalf("Error during configuration of TraceProvider %q: %s", *k.config.TraceProvider, err.Error())
		}

		return provider
	}
	return telemetry.NewNoopTraceProvider()
}

func (k *Kelon) makeConfigWatcher(configLoader configs.FileConfigLoader, configWatcherPath *string) {
	if k.config.RegoDir == nil || *k.config.RegoDir == "" {
		k.configWatcher = watcherInt.NewSimple(configLoader)
	} else {
		// Set configWatcherPath to rego path by default
		if configWatcherPath == nil || *configWatcherPath == "" {
			configWatcherPath = k.config.RegoDir
		}
		k.configWatcher = watcherInt.NewFileWatcher(configLoader, *configWatcherPath)
	}
}

func (k *Kelon) startNewRestProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	// Create Rest proxy and start
	k.proxy = apiInt.NewRestProxy(*k.config.PathPrefix, int32(*k.config.Port))
	if err := k.proxy.Configure(ctx, appConfig, serverConf); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
	// Start proxy
	if err := k.proxy.Start(); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
}

func (k *Kelon) startNewEnvoyProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	if *k.config.EnvoyPort == *k.config.Port {
		logging.LogForComponent("main").Panic("Cannot start envoyProxy proxy and rest proxy on same port!")
	}

	// Create Rest proxy and start
	k.envoyProxy = envoy.NewEnvoyProxy(envoy.Config{
		Port:                   *k.config.EnvoyPort,
		DryRun:                 *k.config.EnvoyDryRun,
		EnableReflection:       *k.config.EnvoyReflection,
		AccessDecisionLogLevel: *k.config.AccessDecisionLogLevel,
	})
	if err := k.envoyProxy.Configure(ctx, appConfig, serverConf); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
	// Start proxy
	if err := k.envoyProxy.Start(); err != nil {
		logging.LogForComponent("main").Fatalln(err.Error())
	}
}

func (k *Kelon) makeServerConfig(compiler opa.PolicyCompiler, parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        k.config.PathPrefix,
			OpaConfigPath: k.config.OpaPath,
			RegoDir:       k.config.RegoDir,
			ConfigWatcher: &k.configWatcher,
			PathProcessor: &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores:   data.MakeDatastores(loadedConf.Data, k.config.Validate),
				SkipUnknown:  *k.config.AstSkipUnknown,
				ValidateMode: k.config.Validate,
			},
			AccessDecisionLogLevel: strings.ToUpper(*k.config.AccessDecisionLogLevel),
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

func (k *Kelon) stopOnSIGTERM() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	// Block until we receive our signal.
	<-interruptChan

	logging.LogForComponent("main").Infoln("Caught SIGTERM...")

	// Stop metrics provider
	// This is done blocking to ensure all metrics are sent!
	k.metricsProvider.Shutdown(context.Background())

	// Stop trace provider
	// This is done blocking to ensure all traces are sent!
	k.traceProvider.Shutdown(context.Background())

	// Stop envoyProxy proxy if started
	if k.envoyProxy != nil {
		if err := k.envoyProxy.Stop(time.Second * 10); err != nil {
			logging.LogForComponent("main").Warnln(err.Error())
		}
	}

	// Stop rest proxy if started
	if k.proxy != nil {
		if err := k.proxy.Stop(time.Second * 10); err != nil {
			logging.LogForComponent("main").Warnln(err.Error())
		}
	}
	// Give components enough time for graceful shutdown
	// This terminates earlier, because rest-proxy prints FATAL if http-server is closed
	time.Sleep(5 * time.Second)
}
