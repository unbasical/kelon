package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/core"
	"gopkg.in/yaml.v3"
)

type testConfiguration struct {
	e2eConfig     ContainerConfiguration
	kelonPort     uint32
	dsConfigPath  string
	apiConfigPath string
	policiesPath  string
	opaConfigPath string
	callOpsPath   string
	requestPath   string
	pathPrefix    string
}

func Test_e2e_kelon(t *testing.T) {
	ctx := context.Background()

	config := testConfiguration{
		e2eConfig: ContainerConfiguration{
			exposePorts:    map[ServiceID][]string{},
			waitStrategies: map[ServiceID]wait.Strategy{},
		},
		kelonPort:     8181,
		dsConfigPath:  "../../examples/docker-compose/config/datastore.yml",
		apiConfigPath: "../../examples/docker-compose/config/api.yml",
		opaConfigPath: "../../examples/docker-compose/config/datastore.yml",
		policiesPath:  "../../examples/docker-compose/policies/",
		callOpsPath:   "../../call-operands",
		requestPath:   "./test_config/requests.yml",
		pathPrefix:    "/v1",
	}

	configureExposablePorts(t, &config)

	env := NewTest(ctx, t, "E2E Tests", &config)

	env.startKelon()

	env.waitForKelon()

	env.runTests()
}

type E2ETestEnvironment struct {
	name         string
	t            *testing.T
	containerEnv *ContainerEnvironment
	kelonPort    uint32
	apiPath      string
	dsPath       string
	regoPath     string
	opaPath      string
	callOpsPath  string
	pathPrefix   string
	requests     []Request
}

func NewTest(ctx context.Context, t *testing.T, name string, config *testConfiguration) *E2ETestEnvironment {
	container := newE2EEnvironment()
	container.Configure(config.e2eConfig)

	t.Cleanup(func() { container.Stop(ctx) })

	if containerErr := container.Start(ctx); containerErr != nil {
		t.Errorf("error starting container environment: %s", containerErr.Error())
		t.FailNow()
	}

	dsPorts := map[ServiceID]string{}
	for service, portMap := range container.exposedPorts {
		dsPorts[service] = portMap[container.portsToExpose[service][0]]
	}

	return &E2ETestEnvironment{
		name:         name,
		t:            t,
		containerEnv: container,
		kelonPort:    config.kelonPort,
		apiPath:      config.apiConfigPath,
		dsPath:       modifyDatastoreConfig(ctx, t, container, config.dsConfigPath, dsPorts),
		opaPath:      config.opaConfigPath,
		regoPath:     config.policiesPath,
		callOpsPath:  config.callOpsPath,
		pathPrefix:   config.pathPrefix,
		requests:     parseTestData(t, config.requestPath),
	}
}

func (env *E2ETestEnvironment) runTests() {
	for _, request := range env.requests {
		url := fmt.Sprintf(request.URL, "localhost", strconv.Itoa(int(env.kelonPort)))

		//nolint:gosec,gocritic
		resp, httpErr := http.Post(url, "application/json", bytes.NewBufferString(request.Body))
		if httpErr != nil {
			env.t.Errorf("%s: %s - %s", request.Name, url, httpErr.Error())
			env.t.FailNow()
		}

		_ = resp.Body.Close()

		fmt.Printf("Name: %s - Expect: %d - Got: %d\n", request.Name, request.StatusCode, resp.StatusCode)

		assert.Equal(env.t, request.StatusCode, resp.StatusCode, "%s: asserting response status code", request.Name)
	}
}

func (env *E2ETestEnvironment) startKelon() {
	env.t.Cleanup(func() {
		env.stopKelon()
	})

	var defaultAccessLogLevel = "ALL"
	var astSkipUnknown = false

	config := core.KelonConfiguration{
		DatastorePath:          &env.dsPath,
		APIPath:                &env.apiPath,
		OpaPath:                &env.opaPath,
		RegoDir:                &env.regoPath,
		Port:                   &env.kelonPort,
		OperandDir:             &env.callOpsPath,
		PathPrefix:             &env.pathPrefix,
		AccessDecisionLogLevel: &defaultAccessLogLevel,
		AstSkipUnknown:         &astSkipUnknown,
	}

	kelon := core.Kelon{}
	kelon.Configure(&config)

	go func() {
		kelon.Start()
	}()
}

func (env *E2ETestEnvironment) waitForKelon() {
	healthy := false
	for !healthy {
		resp, httpErr := http.Get(fmt.Sprintf("http://localhost:%d/health", env.kelonPort))
		if httpErr == nil {
			if resp.StatusCode == http.StatusOK {
				healthy = true
			}
			_ = resp.Body.Close()
		}
	}
}

func (env *E2ETestEnvironment) stopKelon() {
	err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1) // Use signal that is not sent by any other process
	if err != nil {
		env.t.FailNow()
	}
}

func parseTestData(t *testing.T, path string) []Request {
	var data []Request
	inputBytes, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("error reading file %s: %s", path, err.Error())
		t.FailNow()
	}

	err = yaml.Unmarshal(inputBytes, &data)
	if err != nil {
		t.Errorf("error parsing file %s: %s", path, err.Error())
		t.FailNow()
	}
	return data
}

func modifyDatastoreConfig(ctx context.Context, t *testing.T, containerEnv *ContainerEnvironment, path string, ports map[ServiceID]string) string {
	var data configs.DatastoreConfig
	inputBytes, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("error reading file %s: %s", path, err.Error())
		t.FailNow()
	}

	err = yaml.Unmarshal(inputBytes, &data)
	if err != nil {
		t.Errorf("error parsing file %s: %s", path, err.Error())
		t.FailNow()
	}

	for ds, port := range ports {
		c, ok := data.Datastores[string(ds)]
		if !ok {
			t.Errorf("error no config for datastore %s in file %s", ds, path)
			t.FailNow()
		}

		host, hErr := containerEnv.Host(ctx, ds)
		if hErr != nil {
			t.Errorf("unable to get container ip")
			t.FailNow()
		}
		c.Connection["host"] = host
		c.Connection["port"] = port
	}

	dir, err := os.MkdirTemp("", "kelon_e2e")
	if err != nil {
		t.Errorf("error creating tmp dir: %s", err.Error())
		t.FailNow()
	}

	fPath := fmt.Sprintf("%s/dsConfig", dir)
	file, err := os.Create(fPath)
	if err != nil {
		t.Errorf("error creating file %s: %s", fPath, err.Error())
		t.FailNow()
	}

	defer file.Close()

	out, err := yaml.Marshal(data)
	if err != nil {
		t.Errorf("error serializing config: %s", err.Error())
		t.FailNow()
	}

	_, err = file.Write(out)
	if err != nil {
		t.Errorf("error writing to file %s: %s", fPath, err.Error())
		t.FailNow()
	}

	return fPath
}

func configureExposablePorts(t *testing.T, config *testConfiguration) {
	var data configs.DatastoreConfig
	inputBytes, err := os.ReadFile(config.dsConfigPath)
	if err != nil {
		t.Errorf("error reading file %s: %s", config.dsConfigPath, err.Error())
		t.FailNow()
	}

	err = yaml.Unmarshal(inputBytes, &data)
	if err != nil {
		t.Errorf("error parsing file %s: %s", config.dsConfigPath, err.Error())
		t.FailNow()
	}

	for serivce, ds := range data.Datastores {
		id, idErr := serviceFromString(serivce)
		if idErr != nil {
			t.Errorf("error extracting ports from config: %s", err.Error())
			t.FailNow()
		}

		config.e2eConfig.exposePorts[id] = []string{ds.Connection["port"]}
	}
}
