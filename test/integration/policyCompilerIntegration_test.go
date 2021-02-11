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

	"github.com/Foundato/kelon/configs"
	dataInt "github.com/Foundato/kelon/internal/pkg/data"
	opa2 "github.com/Foundato/kelon/internal/pkg/opa"
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

func TestMain(m *testing.M) {
	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestPolicyCompiler(t *testing.T) {
	// change root path for files
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}

	// init configloader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: "./examples/local/config/datastore.yml",
		APIConfigPath:       "./examples/local/config/api.yml",
	}

	// init configwatcher to setup policycompiler
	newConfigWatcher := watcherInt.NewFileWatcher(configLoader, "./examples/local/policies")
	var compiler opa.PolicyCompiler
	newConfigWatcher.Watch(
		func(changeType watcher.ChangeType, config *configs.ExternalConfig, err error) {
			onConfigLoaded(changeType, config, err, &compiler, &newConfigWatcher, t)
		})

	// open and parse policycompiler test requests
	requests := &DBTranslatorRequests{}
	inputBytes, err := ioutil.ReadFile("./test/integration/config/dbRequests.yml")
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
		compiler.ServeHTTP(w, kelonRequest)

		// assert response status
		resp := w.Result()
		statusCode := requests.Requests[strconv.Itoa(counter)].ResponseStatus
		if strconv.Itoa(resp.StatusCode) != statusCode {
			t.Error(err)
			t.FailNow()
		}

		// close response body and increase counter for next iteration
		_ = resp.Body.Close()
		counter++
	}
}

func onConfigLoaded(change watcher.ChangeType, loadedConf *configs.ExternalConfig, err error, pCompiler *opa.PolicyCompiler, configWatcher *watcher.ConfigWatcher, t *testing.T) {
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if change == watcher.ChangeAll {
		// Configure application
		var (
			config     = new(configs.AppConfig)
			parser     = requestInt.NewURLProcessor()
			mapper     = requestInt.NewPathMapper()
			translator = translateInt.NewAstTranslator()
		)

		*pCompiler = opa2.NewPolicyCompiler()

		// Build config
		config.API = loadedConf.API
		config.Data = loadedConf.Data
		serverConf := makeServerConfig(*pCompiler, parser, mapper, translator, loadedConf, configWatcher, t)
		if configErr := (*pCompiler).Configure(config, &serverConf.PolicyCompilerConfig); configErr != nil {
			t.Error(err)
			t.FailNow()
		}
	}
}

func makeServerConfig(compiler opa.PolicyCompiler, parser request.PathProcessor, mapper request.PathMapper, translator translate.AstTranslator, loadedConf *configs.ExternalConfig, configWatcher *watcher.ConfigWatcher, t *testing.T) api.ClientProxyConfig {
	pathPrefix := "/v1"
	opaPath := "./examples/local/config/opa.yml"
	regoDir := "./examples/local/policies"

	// Build server config
	serverConf := api.ClientProxyConfig{
		Compiler: &compiler,
		PolicyCompilerConfig: opa.PolicyCompilerConfig{
			RespondWithStatusCode: false,
			Prefix:                &pathPrefix,
			OpaConfigPath:         &opaPath,
			RegoDir:               &regoDir,
			ConfigWatcher:         configWatcher,
			PathProcessor:         &parser,
			PathProcessorConfig: request.PathProcessorConfig{
				PathMapper: &mapper,
			},
			Translator: &translator,
			AstTranslatorConfig: translate.AstTranslatorConfig{
				Datastores: mockMakeDatastores(loadedConf.Data, t),
			},
			AccessDecisionLogLevel: strings.ToUpper("ALL"),
		},
	}
	return serverConf
}

func mockMakeDatastores(config *configs.DatastoreConfig, t *testing.T) map[string]*data.DatastoreTranslator {
	result := make(map[string]*data.DatastoreTranslator)
	// create and insert mocked db executor into all datastores
	mocked := NewMockedDatastoreExecuter(t)
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
