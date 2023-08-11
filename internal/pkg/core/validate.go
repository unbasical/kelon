package core

import (
	"context"
	"os"

	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/data"
	opaInt "github.com/unbasical/kelon/internal/pkg/opa"
	requestInt "github.com/unbasical/kelon/internal/pkg/request"
	translateInt "github.com/unbasical/kelon/internal/pkg/translate"
	"github.com/unbasical/kelon/pkg/telemetry"
)

func (k *Kelon) StartValidate() {
	if !k.configured {
		k.logger.Fatalf("Kelon was not configured! Please call Configure()!")
	}

	k.makeConfigWatcher(configs.FileConfigLoader{}, k.config.ConfigWatcherPath)

	confLoader := configs.FileConfigLoader{FilePath: *k.config.ConfigPath}
	loadedConf, err := confLoader.Load()
	if err != nil {
		k.logger.Fatalln("Unable to parse configuration: ", err.Error())
	}

	// Create logging file
	if k.config.QueryOutputFilename != nil && *k.config.QueryOutputFilename != "" {
		f, fileErr := os.Create(*k.config.QueryOutputFilename)
		if fileErr != nil {
			k.logger.Fatalln("Unable to parse crate file: ", fileErr.Error())
		}

		defer func(f *os.File) {
			_ = f.Close()
		}(f)

		k.dsLoggingWriter = f
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
	config.APIMappings = loadedConf.APIMappings
	config.DatastoreSchemas = loadedConf.DatastoreSchemas
	config.Datastores = loadedConf.Datastores
	config.OPA = loadedConf.OPA
	config.MetricsProvider = telemetry.NewNoopMetricProvider()
	k.metricsProvider = config.MetricsProvider // Stopped gracefully later on
	config.TraceProvider = telemetry.NewNoopTraceProvider()
	k.traceProvider = config.TraceProvider // Stopped gracefully later on

	serverConf := k.makeServerConfig(compiler, parser, mapper, translator, loadedConf)

	config.CallOperands, err = data.LoadAllCallOperands(config.Datastores, k.config.OperandDir)
	if err != nil {
		k.logger.Fatalln(err.Error())
	}

	err = (*serverConf.Compiler).Configure(config, &serverConf.PolicyCompilerConfig)
	if err != nil {
		k.logger.Fatalln(err.Error())
	}

	body, err := parseBodyFromString(*k.config.InputBody)
	if err != nil {
		k.logger.Fatalln(err.Error())
	}

	// Execute Request
	_, err = compiler.Execute(ctx, body)
	if err != nil {
		k.logger.Fatalln(err.Error())
	}
}
