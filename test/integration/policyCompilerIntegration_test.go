package integration

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"testing"

	opa2 "github.com/Foundato/kelon/internal/pkg/opa"

	"github.com/Foundato/kelon/configs"
	dataInt "github.com/Foundato/kelon/internal/pkg/data"
	requestInt "github.com/Foundato/kelon/internal/pkg/request"
	translateInt "github.com/Foundato/kelon/internal/pkg/translate"
	watcherInt "github.com/Foundato/kelon/internal/pkg/watcher"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/Foundato/kelon/pkg/constants/logging"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/Foundato/kelon/pkg/opa"
	"github.com/Foundato/kelon/pkg/request"
	"github.com/Foundato/kelon/pkg/translate"
	"github.com/Foundato/kelon/pkg/watcher"
	"gopkg.in/yaml.v3"
)

func Test_integration_policyCompiler(t *testing.T) {
	type fields struct {
		dsConfigPath         string
		apiConfigPath        string
		policiesPath         string
		evaluatedQueriesPath string
		requestPath          string
		opaPath              string
		pathPrefix           string
	}

	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "Test",
			fields: fields{
				"./examples/local/config/datastore.yml",
				"./examples/local/config/api.yml",
				"./examples/local/policies",
				"./test/integration/config/dbQueries.yml",
				"./test/integration/config/dbRequests.yml",
				"./examples/local/config/opa.yml",
				"/v1",
			},
		},
	}
	for _, tt := range tests {
		// redefining scope variable for to bypass parallel execution error
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			runPolicyCompilerTest(t, tt.name, tt.fields.dsConfigPath, tt.fields.apiConfigPath, tt.fields.policiesPath, tt.fields.evaluatedQueriesPath, tt.fields.requestPath, tt.fields.opaPath, tt.fields.pathPrefix)
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

func runPolicyCompilerTest(t *testing.T, testName, dsConfigPath, apiConfigPath, policiesPath, evaluatedQueriesPath, requestPath, opaPath, pathPrefix string) {
	// change root path for files
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Errorf("error while changing root for test %s", testName)
		t.FailNow()
	}
	dir := path.Join(path.Dir(filename), "../..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}

	// init configloader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: dsConfigPath,
		APIConfigPath:       apiConfigPath,
	}

	// init policyCompiler to setup configWatcher
	testEnvironment := PolicyCompilerTestEnvironment{
		name:                 testName,
		policyCompiler:       opa2.NewPolicyCompiler(),
		configWatcher:        watcherInt.NewFileWatcher(configLoader, policiesPath),
		opaPath:              opaPath,
		pathPrefix:           pathPrefix,
		policiesPath:         policiesPath,
		evaluatedQueriesPath: evaluatedQueriesPath,
	}

	testEnvironment.configWatcher.Watch(
		func(changeType watcher.ChangeType, config *configs.ExternalConfig, err error) {
			testEnvironment.onConfigLoaded(changeType, config, err)
		})

	// open and parse policycompiler test requests
	requests := &DBTranslatorRequests{}
	inputBytes, err := ioutil.ReadFile(requestPath)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	err = yaml.Unmarshal(inputBytes, requests)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	var kelonRequest *http.Request
	counter := 0

	// send test http requests to policy compiler and evaluate response code
	// db queries are tested in mocked db executor
	for counter < len(requests.Requests) {
		// create and http requests
		body := requests.Requests[strconv.Itoa(counter)].Body
		kelonRequest, err = http.NewRequest("POST", "/v1/data", strings.NewReader(body))
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		w := httptest.NewRecorder()
		testEnvironment.policyCompiler.ServeHTTP(w, kelonRequest)

		// assert response status
		resp := w.Result()
		statusCode := requests.Requests[strconv.Itoa(counter)].ResponseStatus
		if strconv.Itoa(resp.StatusCode) != statusCode {
			t.Errorf("Response status %s does not match expected status: %s in %s", strconv.Itoa(resp.StatusCode), statusCode, testName)
			t.FailNow()
		}

		// close response body and increase counter for next iteration
		_ = resp.Body.Close()
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
			config     = new(configs.AppConfig)
			parser     = requestInt.NewURLProcessor()
			mapper     = requestInt.NewPathMapper()
			translator = translateInt.NewAstTranslator()
		)

		// Build config
		config.API = loadedConf.API
		config.Data = loadedConf.Data
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
			RespondWithStatusCode: false,
			Prefix:                &pathPrefix,
			OpaConfigPath:         &opaPath,
			RegoDir:               &regoDir,
			ConfigWatcher:         &p.configWatcher,
			PathProcessor:         &parser,
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

func (p *PolicyCompilerTestEnvironment) mockMakeDatastores(config *configs.DatastoreConfig) map[string]*data.DatastoreTranslator {
	result := make(map[string]*data.DatastoreTranslator)
	// create and insert mocked db executor into all datastores
	mocked := NewMockedDatastoreExecuter(p.t, p.evaluatedQueriesPath, p.name)
	for dsName, ds := range config.Datastores {
		if ds.Type == data.TypeMysql || ds.Type == data.TypePostgres {
			newDs := dataInt.NewSQLDatastore(mocked)
			logging.LogForComponent("factory").Infof("Init Datastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
		}
		if ds.Type == data.TypeMongo {
			newDs := dataInt.NewMongoDatastore(mocked)
			logging.LogForComponent("factory").Infof("Init MongoDatastore of type [%s] with alias [%s]", ds.Type, dsName)
			result[dsName] = &newDs
			continue
		}
	}
	return result
}
