package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/Foundato/kelon/pkg/opa"
	"github.com/open-policy-agent/opa/util"

	ext_authz "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"google.golang.org/genproto/googleapis/rpc/code"
)

const exampleAllowedRequest = `{
	"attributes": {
	  "request": {
		"http": {
		  "id": "13359530607844510314",
		  "method": "GET",
		  "headers": {
			":authority": "192.168.99.100:31380",
			":method": "GET",
			":path": "/api/v1/products",
			"accept": "*/*",
			"authorization": "Basic Ym9iOnBhc3N3b3Jk",
			"content-length": "0",
			"user-agent": "curl/7.54.0",
			"x-b3-sampled": "1",
			"x-b3-spanid": "537f473f27475073",
			"x-b3-traceid": "537f473f27475073",
			"x-envoy-internal": "true",
			"x-forwarded-for": "172.17.0.1",
			"x-forwarded-proto": "http",
			"x-istio-attributes": "Cj4KE2Rlc3RpbmF0aW9uLnNlcnZpY2USJxIlcHJvZHVjdHBhZ2UuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbApPCgpzb3VyY2UudWlkEkESP2t1YmVybmV0ZXM6Ly9pc3Rpby1pbmdyZXNzZ2F0ZXdheS02Nzk5NWM0ODZjLXFwOGpyLmlzdGlvLXN5c3RlbQpBChdkZXN0aW5hdGlvbi5zZXJ2aWNlLnVpZBImEiRpc3RpbzovL2RlZmF1bHQvc2VydmljZXMvcHJvZHVjdHBhZ2UKQwoYZGVzdGluYXRpb24uc2VydmljZS5ob3N0EicSJXByb2R1Y3RwYWdlLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwKKgodZGVzdGluYXRpb24uc2VydmljZS5uYW1lc3BhY2USCRIHZGVmYXVsdAopChhkZXN0aW5hdGlvbi5zZXJ2aWNlLm5hbWUSDRILcHJvZHVjdHBhZ2U=",
			"x-request-id": "92a6c0f7-0250-944b-9cfc-ae10cbcedd8e"
		  },
		  "path": "/api/v1/products",
		  "host": "192.168.99.100:31380",
		  "protocol": "HTTP/1.1",
		  "body": "{\"firstname\": \"foo\", \"lastname\": \"bar\"}"
		}
	  }
	}
  }`

type mockCompiler struct {
	failOnConfigure bool
	failOnProcess   bool
	decision        bool
}

func (c mockCompiler) Configure(appConfig *configs.AppConfig, compConfig *opa.PolicyCompilerConfig) error {
	if c.failOnConfigure {
		return errors.New("Mock config failure ")
	}
	return nil
}

func (c mockCompiler) Process(request *http.Request) (bool, error) {
	if c.failOnProcess {
		return false, errors.New("Mock process failure ")
	}
	return c.decision, nil
}

func TestCheckAllow(t *testing.T) {
	// Example Envoy Check Request for input:
	// curl --user  bob:password  -o /dev/null -s -w "%{http_code}\n" http://${GATEWAY_URL}/api/v1/products

	var req ext_authz.CheckRequest
	if err := util.Unmarshal([]byte(exampleAllowedRequest), &req); err != nil {
		panic(err)
	}

	proxy := NewEnvoyProxy(EnvoyConfig{
		Addr:             ":9191",
		DryRun:           false,
		EnableReflection: true,
	})

	//nolint:gosimple
	var compiler opa.PolicyCompiler
	compiler = mockCompiler{
		failOnConfigure: false,
		failOnProcess:   false,
		decision:        true,
	}
	_ = proxy.Configure(&configs.AppConfig{}, &api.ClientProxyConfig{Compiler: &compiler})
	server, _ := proxy.(*envoyProxy)

	ctx := context.Background()
	output, err := server.envoy.Check(ctx, &req)
	if err != nil {
		t.Fatal(err)
	}
	if output.Status.Code != int32(code.Code_OK) {
		t.Fatal("Expected request to be allowed but got:", output)
	}
}
