package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/unbasical/kelon/internal/pkg/core"
)

const (
	envoyImage        = "docker.io/envoyproxy/envoy:v1.33-latest"
	envoyListenerPort = "8080/tcp"

	// Ports differing from Test_e2e_kelon so both tests can run in one test binary
	kelonEnvoyGrpcPort = 9191
	kelonEnvoyRestPort = 8282
)

// Test_e2e_envoy verifies kelon's ext_authz v3 integration against a real Envoy instance:
// Envoy runs in a container and consults kelon (running in-process on the host) for
// every request via the ext_authz gRPC filter.
func Test_e2e_envoy(t *testing.T) {
	ctx := context.Background()

	pgHost, pgPort := startPostgresContainer(ctx, t)
	t.Setenv("KELON_E2E_PG_HOST", pgHost)
	t.Setenv("KELON_E2E_PG_PORT", pgPort)

	startKelonWithEnvoy(t)
	waitForKelonHealthy(t, kelonEnvoyRestPort)
	waitForTCPPort(t, kelonEnvoyGrpcPort)

	host, port := startEnvoyContainer(ctx, t)

	for _, request := range parseTestData(t, "./test_config/envoy/requests.yml") {
		t.Run(request.Name, func(t *testing.T) {
			url := fmt.Sprintf(request.URL, host, port)

			//nolint:gosec,gocritic
			req, reqErr := http.NewRequest(strings.ToUpper(request.Method), url, bytes.NewBufferString(request.Body))
			if reqErr != nil {
				t.Errorf("%s: %s - %s", request.Name, url, reqErr.Error())
				t.FailNow()
			}

			req.Header.Set("Content-Type", "application/json")

			for k, v := range request.Headers {
				req.Header.Set(k, v)
			}

			resp, httpErr := http.DefaultClient.Do(req)
			if httpErr != nil {
				t.Errorf("%s: %s - %s", request.Name, url, httpErr.Error())
				t.FailNow()
			}

			_ = resp.Body.Close()

			fmt.Printf("Name: %s - Expect: %d - Got: %d\n", request.Name, request.StatusCode, resp.StatusCode)

			assert.Equal(t, request.StatusCode, resp.StatusCode, "%s: asserting response status code", request.Name)
		})
	}
}

// startPostgresContainer starts the datastore required for kelon to boot.
// The envoy E2E policies never query it, so no init scripts are needed.
func startPostgresContainer(ctx context.Context, t *testing.T) (host, port string) {
	req := tc.ContainerRequest{
		Image: "docker.io/postgres:15",
		Env: map[string]string{
			"POSTGRES_DB":       "appstore",
			"POSTGRES_USER":     "You",
			"POSTGRES_PASSWORD": "SuperSecure",
		},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort("5432/tcp"),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("error starting postgres container: %s", err.Error())
	}

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err = container.Host(ctx)
	if err != nil {
		t.Fatalf("unable to get postgres container host: %s", err.Error())
	}

	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("unable to get mapped postgres port: %s", err.Error())
	}

	return host, mappedPort.Port()
}

// startKelonWithEnvoy starts kelon in-process with the envoy ext_authz gRPC server enabled
func startKelonWithEnvoy(t *testing.T) {
	configPath := "./test_config/envoy/kelon.yml"
	regoDir := "./test_config/envoy/policies"
	operandDir := "../../examples/docker-compose/call-operands"
	pathPrefix := "/v1"
	restPort := uint32(kelonEnvoyRestPort)
	envoyPort := uint32(kelonEnvoyGrpcPort)

	config := core.KelonConfiguration{
		ConfigPath:             &configPath,
		RegoDir:                &regoDir,
		Port:                   &restPort,
		OperandDir:             &operandDir,
		PathPrefix:             &pathPrefix,
		AccessDecisionLogLevel: new("ALL"),
		AstSkipUnknown:         new(false),
		EnvoyPort:              &envoyPort,
		EnvoyDryRun:            new(false),
		EnvoyReflection:        new(true),
	}

	kelon := core.Kelon{}
	kelon.Configure(&config)

	go func() {
		kelon.Start()
	}()

	t.Cleanup(func() {
		// Use signal that is not sent by any other process to trigger kelon's graceful shutdown
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	})
}

// startEnvoyContainer starts an Envoy container whose ext_authz filter points to the
// kelon gRPC server on the host (reachable as host.testcontainers.internal)
func startEnvoyContainer(ctx context.Context, t *testing.T) (host, port string) {
	envoyConfPath, err := filepath.Abs("./test_config/envoy/envoy.yaml")
	if err != nil {
		t.Fatalf("unable to resolve envoy config path: %s", err.Error())
	}

	req := tc.ContainerRequest{
		Image:           envoyImage,
		ExposedPorts:    []string{envoyListenerPort},
		HostAccessPorts: []int{kelonEnvoyGrpcPort},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      envoyConfPath,
				ContainerFilePath: "/etc/envoy/envoy.yaml",
				FileMode:          0o644,
			},
		},
		WaitingFor: wait.ForListeningPort(envoyListenerPort),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("error starting envoy container: %s", err.Error())
	}

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err = container.Host(ctx)
	if err != nil {
		t.Fatalf("unable to get envoy container host: %s", err.Error())
	}

	mappedPort, err := container.MappedPort(ctx, envoyListenerPort)
	if err != nil {
		t.Fatalf("unable to get mapped envoy port: %s", err.Error())
	}

	return host, mappedPort.Port()
}

func waitForKelonHealthy(t *testing.T, port uint32) {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, httpErr := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if httpErr == nil {
			healthy := resp.StatusCode == http.StatusOK
			_ = resp.Body.Close()
			if healthy {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("kelon did not become healthy on port %d", port)
}

func waitForTCPPort(t *testing.T, port uint32) {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), time.Second)
		if dialErr == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d did not become reachable", port)
}
