package integration

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"testing"

	opa2 "github.com/unbasical/kelon/internal/pkg/opa"

	"github.com/unbasical/kelon/configs"
	dataInt "github.com/unbasical/kelon/internal/pkg/data"
	requestInt "github.com/unbasical/kelon/internal/pkg/request"
	translateInt "github.com/unbasical/kelon/internal/pkg/translate"
	watcherInt "github.com/unbasical/kelon/internal/pkg/watcher"
	"github.com/unbasical/kelon/pkg/api"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	"github.com/unbasical/kelon/pkg/opa"
	"github.com/unbasical/kelon/pkg/request"
	"github.com/unbasical/kelon/pkg/telemetry"
	"github.com/unbasical/kelon/pkg/translate"
	"github.com/unbasical/kelon/pkg/watcher"
	"gopkg.in/yaml.v3"
)

type testConfiguration struct {
	dsConfigPath         string
	apiConfigPath        string
	policiesPath         string
	evaluatedQueriesPath string
	requestPath          string
	opaPath              string
	pathPrefix           string
}

func Test_integration_policyCompiler(t *testing.T) {
	tests := []struct {
		name   string
		fields testConfiguration
	}{
		{
			name: "Test",
			fields: testConfiguration{
				dsConfigPath:         "./examples/local/config/datastore.yml",
				apiConfigPath:        "./examples/local/config/api.yml",
				policiesPath:         "./examples/local/policies",
				evaluatedQueriesPath: "./test/integration/config/dbQueries.yml",
				requestPath:          "./test/integration/config/dbRequests.yml",
				opaPath:              "./examples/local/config/opa.yml",
				pathPrefix:           "/v1",
			},
		},
	}
	for _, tt := range tests {
		// redefining scope variable for to bypass parallel execution error
		testConfig := tt.fields
		testName := tt.name
		t.Run(tt.name, func(t *testing.T) {
			runPolicyCompilerTest(t, testName, &testConfig)
		})
	}
}

type PolicyCompilerTestEnvironment struct {
	name                 string
	configWatcher        watcher.ConfigWatcher
	policyCompiler       opa.PolicyCompiler
	t                    *testing.T
	opaPath              string
	pathPrefix           string
	policiesPath         string
	evaluatedQueriesPath string
}

func runPolicyCompilerTest(t *testing.T, name string, config *testConfiguration) {
	// change root path for files
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Errorf("error while changing root for test %s", name)
		t.FailNow()
	}
	dir := path.Join(path.Dir(filename), "../..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}

	// init configloader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: config.dsConfigPath,
		APIConfigPath:       config.apiConfigPath,
	}

	// init policyCompiler to set up configWatcher
	testEnvironment := PolicyCompilerTestEnvironment{
		name:                 name,
		policyCompiler:       opa2.NewPolicyCompiler(),
		configWatcher:        watcherInt.NewFileWatcher(configLoader, config.policiesPath),
		opaPath:              config.opaPath,
		pathPrefix:           config.pathPrefix,
		policiesPath:         config.policiesPath,
		evaluatedQueriesPath: config.evaluatedQueriesPath,
	}

	testEnvironment.configWatcher.Watch(
		func(changeType watcher.ChangeType, config *configs.ExternalConfig, err error) {
			testEnvironment.onConfigLoaded(changeType, config, err)
		})

	// open and parse policyCompiler test requests
	requests := &DBTranslatorRequests{}
	inputBytes, err := os.ReadFile(config.requestPath)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	err = yaml.Unmarshal(inputBytes, requests)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	counter := 0

	// send test http requests to policy compiler and evaluate response code
	// db queries are tested in mocked db executor
	for counter < len(requests.Requests) {
		testName := requests.Requests[strconv.Itoa(counter)].Text

		// create and http requests
		bodyStr := requests.Requests[strconv.Itoa(counter)].Body
		var requestBody map[string]interface{}
		err = json.Unmarshal([]byte(bodyStr), &requestBody)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}

		_, respErr := testEnvironment.policyCompiler.Execute(context.Background(), requestBody)

		// If error does not match expected success parameter -> fail
		successExpected := requests.Requests[strconv.Itoa(counter)].Success
		if (successExpected && respErr != nil) || (!successExpected && respErr == nil) {
			if successExpected {
				t.Errorf("Success expected, but go error [%s] in %s: %s", respErr, name, testName)
			} else {
				t.Errorf("Error expected but succeeded in %s: %s", name, testName)
			}

			t.FailNow()
		}
		counter++
	}
}

func (p *PolicyCompilerTestEnvironment) onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error) {
	if err != nil {
		p.t.Error(err)
		p.t.FailNow()
	}

	if change == watcher.ChangeAll {
		// Configure application
		var (
			config = &configs.AppConfig{
				MetricsProvider: telemetry.NewNoopMetricProvider(),
				TraceProvider:   telemetry.NewNoopTraceProvider(),
			}
			parser     = requestInt.NewURLProcessor()
			mapper     = requestInt.NewPathMapper()
			translator = translateInt.NewAstTranslator()
		)

		// Build config
		config.API = loadedConf.API
		config.Data = loadedConf.Data
		config.Data.CallOperandsDir = "./call-operands"
		serverConf := p.makeServerConfig(parser, mapper, translator, loadedConf)
		if configErr := p.policyCompiler.Configure(config, &serverConf.PolicyCompilerConfig); configErr != nil {
			p.t.Error(err)
			p.t.FailNow()
		}
	}
}

func (p *PolicyCompilerTestEnvironment) makeServerConfig(parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	pathPrefix := p.pathPrefix
	opaPath := p.opaPath
	regoDir := p.policiesPath

	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &p.policyCompiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        &pathPrefix,
			OpaConfigPath: &opaPath,
			RegoDir:       &regoDir,
			ConfigWatcher: &p.configWatcher,
			PathProcessor: &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores: p.mockMakeDatastores(loadedConf.Data),
			},
			AccessDecisionLogLevel: strings.ToUpper("ALL"),
		},
	}
	return serverConf
}

func (p *PolicyCompilerTestEnvironment) mockMakeDatastores(config *configs.DatastoreConfig) map[string]*data.Datastore {
	result := make(map[string]*data.Datastore)
	// create and insert mocked db executor into all datastores
	mocked := NewMockedDatastoreExecuter(p.t, p.evaluatedQueriesPath, p.name)
	for dsName, ds := range config.Datastores {
		if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
			newDs := dataInt.NewDatastore(dataInt.NewSQLDatastoreTranslator(), mocked)
			logging.LogForComponent("factory").Infof("Init Datastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		}
		if ds.Type == data.TypeMongo {
			newDs := dataInt.NewDatastore(dataInt.NewMongoDatastoreTranslator(), mocked)
			logging.LogForComponent("factory").Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}
	}
	return result
}
