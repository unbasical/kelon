package main

import (
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/common"
	"github.com/unbasical/kelon/internal/pkg/core"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"gopkg.in/alecthomas/kingpin.v2"
)

//nolint:gochecknoglobals,gocritic
var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")

	// Commands
	run      = app.Command("run", "Run kelon in production mode.")
	validate = app.Command("validate", "Run kelon in validate mode: validate policies by printing resulting datastore queries")

	// Config paths
	configurationPath = app.Flag("config", "Path to the configuration yaml.").Short('k').Default("./kelon.yml").Envar("KELON_CONF").ExistingFile()
	configWatcherPath = app.Flag("config-watcher-path", "Path where the config watcher should listen for changes.").Envar("CONFIG_WATCHER_PATH").ExistingDir()
	regoDir           = app.Flag("rego-dir", "Dir containing .rego files which will be loaded into OPA.").Short('r').Envar("REGO_DIR").ExistingDir()
	operandDir        = app.Flag("call-operand-dir", "Dir containing .yaml files which contain the call operand configuration for the datastores").Short('c').Envar("CALL_OPERANDS_DIR").ExistingDir()
	extensionsDir     = app.Flag("extension-dir", "Dir containing .so plugin binaries which should be loaded as extensions").Short('e').Envar("EXTENSION_DIR").ExistingDir()

	// Additional config
	pathPrefix     = app.Flag("path-prefix", "Prefix which is used to proxy OPA's Data-API.").Default("/v1").Envar("PATH_PREFIX").String()
	port           = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Uint32()
	astSkipUnknown = app.Flag("ast-skip-unknown", "Skip unknown parts in the AST and only log as warning.").Default("false").Envar("AST_SKIP_UNKNOWN").Bool()

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
)

func main() {
	app.HelpFlag.Short('h')
	app.Version(common.Version)

	// Process args and initialize logger
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	log.SetOutput(os.Stdout)
	// Set log format
	setLogFormat()
	// Set log level
	setLogLevel()

	config := core.KelonConfiguration{
		ConfigPath:             configurationPath,
		ConfigWatcherPath:      configWatcherPath,
		RegoDir:                regoDir,
		OperandDir:             operandDir,
		ExtensionDir:           extensionsDir,
		PathPrefix:             pathPrefix,
		Port:                   port,
		AstSkipUnknown:         astSkipUnknown,
		AccessDecisionLogLevel: accessDecisionLogLevel,
		EnvoyPort:              envoyPort,
		EnvoyDryRun:            envoyDryRun,
		EnvoyReflection:        envoyReflection,
		MetricService:          metricService,
		MetricExportProtocol:   metricExportProtocol,
		MetricExportEndpoint:   metricExportEndpoint,
		TraceService:           traceService,
		TraceExportProtocol:    traceExportProtocol,
		TraceExportEndpoint:    traceExportEndpoint,
		Validate:               false,
		InputBody:              inputBody,
		QueryOutputFilename:    queryOutputFilename,
	}

	kelon := core.Kelon{}

	switch cmd {
	case run.FullCommand():
		kelon.Configure(&config)
		kelon.Start()

	case validate.FullCommand():
		config.Validate = true
		kelon.Configure(&config)
		kelon.StartValidate()

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
