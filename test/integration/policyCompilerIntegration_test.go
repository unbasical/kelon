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

	"github.com/unbasical/kelon/configs"
	dataInt "github.com/unbasical/kelon/internal/pkg/data"
	opa2 "github.com/unbasical/kelon/internal/pkg/opa"
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
	configPath           string
	policiesPath         string
	callOpsPath          string
	evaluatedQueriesPath string
	requestPath          string
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
				configPath:           "./examples/local/config/kelon.yml",
				policiesPath:         "./examples/local/policies",
				callOpsPath:          "./examples/local/call-operands",
				evaluatedQueriesPath: "./test/integration/config/dbQueries.yml",
				requestPath:          "./test/integration/config/dbRequests.yml",
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
	pathPrefix           string
	policiesPath         string
	callOpsPath          string
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
		FilePath: config.configPath,
	}

	// init policyCompiler to set up configWatcher
	testEnvironment := PolicyCompilerTestEnvironment{
		name:                 name,
		t:                    t,
		policyCompiler:       opa2.NewPolicyCompiler(),
		configWatcher:        watcherInt.NewFileWatcher(configLoader, config.policiesPath),
		pathPrefix:           config.pathPrefix,
		policiesPath:         config.policiesPath,
		callOpsPath:          config.callOpsPath,
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
		var requestBody map[string]any
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
		logging.LogForComponent("mockedDatastoreExecuter").Infof("PASS: %s", testName)
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
		config.APIMappings = loadedConf.APIMappings
		config.Datastores = loadedConf.Datastores
		config.DatastoreSchemas = loadedConf.DatastoreSchemas
		serverConf := p.makeServerConfig(parser, mapper, translator, loadedConf)
		config.CallOperands, err = dataInt.LoadAllCallOperands(config.Datastores, &p.callOpsPath)
		if err != nil {
			p.t.Error(err)
			p.t.FailNow()
		}

		if configErr := p.policyCompiler.Configure(config, &serverConf.PolicyCompilerConfig); configErr != nil {
			p.t.Error(configErr)
			p.t.FailNow()
		}
	}
}

func (p *PolicyCompilerTestEnvironment) makeServerConfig(parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig) api.ClientProxyConfig {
	pathPrefix := p.pathPrefix
	regoDir := p.policiesPath

	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &p.policyCompiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			Prefix:        &pathPrefix,
			RegoDir:       &regoDir,
			ConfigWatcher: &p.configWatcher,
			PathProcessor: &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores: p.mockMakeDatastores(loadedConf),
			},
			AccessDecisionLogLevel: strings.ToUpper("ALL"),
		},
	}
	return serverConf
}

func (p *PolicyCompilerTestEnvironment) mockMakeDatastores(config *configs.ExternalConfig) map[string]*data.Datastore {
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
