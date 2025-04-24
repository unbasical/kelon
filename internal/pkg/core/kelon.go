package core

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	apiInt "github.com/unbasical/kelon/internal/pkg/api"
	"github.com/unbasical/kelon/internal/pkg/api/envoy"
	"github.com/unbasical/kelon/internal/pkg/builtins"
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
	ConfigPath        *string
	ConfigWatcherPath *string
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
	logger          *log.Entry
	config          *KelonConfiguration
	dsLoggingWriter io.Writer
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

	k.logger = logging.LogForComponent("main")

	// Configure opa builtins
	builtins.RegisterLoggingFunctions()

	k.config = config

	k.configured = true
}

func (k *Kelon) Start() {
	if !k.configured {
		k.logger.Fatalf("Kelon was not configured! Please call Configure()!")
	}

	// Init config loader
	configLoader := configs.FileConfigLoader{
		FilePath: *k.config.ConfigPath,
	}

	// Start app after config is present
	k.makeConfigWatcher(configLoader, k.config.ConfigWatcherPath)
	k.configWatcher.Watch(k.onConfigLoaded)
	k.stopOnSIGTERM()
}

func (k *Kelon) onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		k.logger.Fatalln("Unable to parse configuration: ", err.Error())
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
		config.Global = loadedConf.Global
		config.APIMappings = loadedConf.APIMappings
		config.DatastoreSchemas = loadedConf.DatastoreSchemas
		config.Datastores = loadedConf.Datastores
		config.OPA = loadedConf.OPA
		config.MetricsProvider = k.makeTelemetryMetricsProvider(ctx)
		k.metricsProvider = config.MetricsProvider // Stopped gracefully later on
		config.TraceProvider = k.makeTelemetryTraceProvider(ctx)
		k.traceProvider = config.TraceProvider // Stopped gracefully later on

		serverConf := k.makeServerConfig(compiler, parser, mapper, translator, loadedConf)

		// load call operands for the datastore translator
		k.loadCallOperands(config)

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
			k.logger.Fatalf("Error during creation of MetricsProvider %q: %s", *k.config.MetricProvider, err)
		}

		if err := provider.Configure(ctx); err != nil {
			k.logger.Fatalf("Error during configuration of MetricsProvider %q: %s", *k.config.MetricProvider, err.Error())
		}

		return provider
	}
	return telemetry.NewNoopMetricProvider()
}

func (k *Kelon) makeTelemetryTraceProvider(ctx context.Context) telemetry.TraceProvider {
	if k.config.TraceProvider != nil && *k.config.TraceProvider != "" {
		provider, err := telemetry.NewTraceProvider(ctx, *k.config.OtlpServiceName, *k.config.OtlpTraceExportProtocol, *k.config.OtlpTraceExportEndpoint)
		if err != nil {
			k.logger.Fatalf("Error during creation of TraceProvider %q: %s", *k.config.TraceProvider, err)
		}

		if err := provider.Configure(ctx); err != nil {
			k.logger.Fatalf("Error during configuration of TraceProvider %q: %s", *k.config.TraceProvider, err.Error())
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

func (k *Kelon) loadCallOperands(appConfig *configs.AppConfig) {
	ops, err := data.LoadAllCallOperands(appConfig.Datastores, k.config.OperandDir)
	if err != nil {
		k.logger.Fatalln(err.Error())
	}
	appConfig.CallOperands = ops
}

func (k *Kelon) startNewRestProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	// Create Rest proxy and start
	k.proxy = apiInt.NewRestProxy(*k.config.PathPrefix, *k.config.Port)
	if err := k.proxy.Configure(ctx, appConfig, serverConf); err != nil {
		k.logger.Fatalln(err.Error())
	}
	// Start proxy
	if err := k.proxy.Start(); err != nil {
		k.logger.Fatalln(err.Error())
	}
}

func (k *Kelon) startNewEnvoyProxy(ctx context.Context, appConfig *configs.AppConfig, serverConf *api.ClientProxyConfig) {
	if *k.config.EnvoyPort == *k.config.Port {
		k.logger.Panic("Cannot start envoyProxy proxy and rest proxy on same port!")
	}

	// Create Rest proxy and start
	k.envoyProxy = envoy.NewEnvoyProxy(envoy.Config{
		Port:                   *k.config.EnvoyPort,
		DryRun:                 *k.config.EnvoyDryRun,
		EnableReflection:       *k.config.EnvoyReflection,
		AccessDecisionLogLevel: *k.config.AccessDecisionLogLevel,
	})
	if err := k.envoyProxy.Configure(ctx, appConfig, serverConf); err != nil {
		k.logger.Fatalln(err.Error())
	}
	// Start proxy
	if err := k.envoyProxy.Start(); err != nil {
		k.logger.Fatalln(err.Error())
	}
}

func (k *Kelon) makeServerConfig(compiler opa.PolicyCompiler, parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        k.config.PathPrefix,
			RegoDir:       k.config.RegoDir,
			OPAConfig:     loadedConf.OPA,
			ConfigWatcher: &k.configWatcher,
			PathProcessor: &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores:   data.MakeDatastores(loadedConf, k.dsLoggingWriter, k.config.Validate),
				SkipUnknown:  *k.config.AstSkipUnknown,
				ValidateMode: k.config.Validate,
			},
			AccessDecisionLogLevel: strings.ToUpper(*k.config.AccessDecisionLogLevel),
		},
	}
	return serverConf
}

func parseBodyFromString(input string) (map[string]any, error) {
	in := []byte(input)
	var raw map[string]any

	err := json.Unmarshal(in, &raw)
	return raw, err
}

func (k *Kelon) stopOnSIGTERM() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	// Block until we receive our signal.
	<-interruptChan

	k.logger.Infoln("Caught SIGTERM...")

	// Stop metrics provider
	// This is done blocking to ensure all metrics are sent!
	k.metricsProvider.Shutdown(context.Background())

	// Stop trace provider
	// This is done blocking to ensure all traces are sent!
	k.traceProvider.Shutdown(context.Background())

	// Stop envoyProxy proxy if started
	if k.envoyProxy != nil {
		if err := k.envoyProxy.Stop(time.Second * 10); err != nil {
			k.logger.Warnln(err.Error())
		}
	}

	// Stop rest proxy if started
	if k.proxy != nil {
		if err := k.proxy.Stop(time.Second * 10); err != nil {
			k.logger.Warnln(err.Error())
		}
	}
	// Give components enough time for graceful shutdown
	// This terminates earlier, because rest-proxy prints FATAL if http-server is closed
	time.Sleep(5 * time.Second)
}
