package envoy

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/Foundato/kelon/pkg/constants/logging"
	"github.com/Foundato/kelon/pkg/telemetry"
	authv2 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/gogo/googleapis/google/rpc"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

// Config represents the plugin configuration.
type Config struct {
	Port                   uint32 `json:"port"`
	DryRun                 bool   `json:"dry-run"`
	AccessDecisionLogLevel string
}

type (
	extAuthzServerV2 struct {
		proxy *envoyGrpcProxy
	}
	extAuthzServerV3 struct {
		proxy *envoyGrpcProxy
	}
)

// Implementation based on guide
// https://github.com/istio/istio/blob/release-1.12/samples/extauthz/src/main.go
type envoyGrpcProxy struct {
	configured bool
	grpcConfig Config
	server     *grpc.Server
	compiler   *http.Handler
	appConf    *configs.AppConfig
	config     *api.ClientProxyConfig
	grpcV2     *extAuthzServerV2
	grpcV3     *extAuthzServerV3
}

func NewEnvoyGrpcProxy(config Config) api.ClientProxy {
	if config.Port == 0 {
		logging.LogForComponent("Config").Warnln("EnvoyProxy was initialized with default properties! You may have missed some arguments when creating it!")
		config.Port = 9191
		config.DryRun = false
	}

	return &envoyGrpcProxy{
		configured: false,
		grpcConfig: config,
	}
}

func (proxy *envoyGrpcProxy) Configure(appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
	// Exit if already configured
	if proxy.configured {
		return nil
	}

	// Configure subcomponents
	if serverConf.Compiler == nil {
		return errors.Errorf("EnvoyProxy: Compiler not configured! ")
	}
	compiler := *serverConf.Compiler
	if err := compiler.Configure(appConf, &serverConf.PolicyCompilerConfig); err != nil {
		return err
	}

	// Configure telemetry (if set)
	handler, err := telemetry.ApplyTelemetryIfPresent(appConf.TelemetryProvider, compiler)
	if err != nil {
		return errors.Wrap(err, "EnvoyProxy encountered error during telemetry provider configuration")
	}

	// Assign variables
	proxy.compiler = &handler
	proxy.appConf = appConf
	proxy.config = serverConf
	proxy.appConf = appConf
	proxy.grpcV2 = &extAuthzServerV2{proxy: proxy}
	proxy.grpcV3 = &extAuthzServerV3{proxy: proxy}
	proxy.configured = true
	logging.LogForComponent("envoyGrpcProxy").Infoln("Configured")
	return nil
}

func (proxy *envoyGrpcProxy) Start() error {
	if !proxy.configured {
		return errors.Errorf("EnvoyProxy was not configured! Please call Configure(). ")
	}

	// Init grpc server
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", proxy.grpcConfig.Port))
	if err != nil {
		return errors.Wrap(err, "Failed to start gRPC server")
	}
	proxy.server = grpc.NewServer()
	authv2.RegisterAuthorizationServer(proxy.server, proxy.grpcV2)
	authv3.RegisterAuthorizationServer(proxy.server, proxy.grpcV3)

	// Start server
	logging.LogForComponent("envoyGrpcProxy").Infof("Starting envoy grpc-server at: %s", listener.Addr())
	go func() {
		if err := proxy.server.Serve(listener); err != nil {
			logging.LogForComponent("envoyGrpcProxy").WithError(err).Fatal("Failed to serve gRPC server")
		}
	}()
	return nil
}

func (proxy *envoyGrpcProxy) Stop(deadline time.Duration) error {
	if proxy.server == nil {
		return errors.Errorf("EnvoyProxy has not bin started yet")
	}

	logging.LogForComponent("envoyGrpcProxy").Info("Stopping envoy grpc-server")
	proxy.server.GracefulStop()
	return nil
}

// Check implements gRPC v2 check request.
func (s *extAuthzServerV2) Check(ctx context.Context, request *authv2.CheckRequest) (*authv2.CheckResponse, error) {
	decision, err := s.proxy.compileOPA(ctx, ExternalRequestV2{
		request: request.GetAttributes().GetRequest().GetHttp(),
	})
	if err != nil {
		return &authv2.CheckResponse{Status: &status.Status{Code: int32(rpc.OK)}}, errors.Wrap(err, "Error while compilation of request with OPA")
	}

	if decision {
		return &authv2.CheckResponse{Status: &status.Status{Code: int32(rpc.OK)}}, nil
	} else {
		return &authv2.CheckResponse{Status: &status.Status{Code: int32(rpc.PERMISSION_DENIED)}}, nil
	}
}

func (s *extAuthzServerV3) Check(ctx context.Context, request *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	decision, err := s.proxy.compileOPA(ctx, ExternalRequestV3{
		request: request.GetAttributes().GetRequest().GetHttp(),
	})
	if err != nil {
		return &authv3.CheckResponse{Status: &status.Status{Code: int32(rpc.OK)}}, errors.Wrap(err, "Error while compilation of request with OPA")
	}

	if decision {
		return &authv3.CheckResponse{Status: &status.Status{Code: int32(rpc.OK)}}, nil
	} else {
		return &authv3.CheckResponse{Status: &status.Status{Code: int32(rpc.PERMISSION_DENIED)}}, nil
	}
}

// Check a new incoming request
func (proxy *envoyGrpcProxy) compileOPA(ctx context.Context, r ExternalRequest) (bool, error) {
	// Rebuild http request
	path := r.GetPath()
	if r.GetQuery() != "" {
		path = fmt.Sprintf("%s?%s", path, r.GetQuery())
	}
	body := r.GetBody()
	if body == "" {
		body = "{}"
	}

	token := ""
	if tokenHeader, ok := r.GetHeaders()["authorization"]; ok {
		token = tokenHeader
	}

	inputBody := fmt.Sprintf(`{
		"input": {
			"method": "%s",
			"path": "%s",
			"token": "%s",
			"payload": %s
		}
	}`, r.GetMethod(), path, token, body)

	httpRequest, err := http.NewRequest("POST", "http://envoy.ext.auth.proxy/v1/data", strings.NewReader(inputBody))
	if err != nil {
		return false, errors.Wrap(err, "EnvoyProxy: Unable to reconstruct HTTP-Request")
	}

	// Set headers
	for headerKey, headerValue := range r.GetHeaders() {
		httpRequest.Header.Set(headerKey, headerValue)
	}

	logging.LogForComponent("envoyExtAuthzGrpcServer").Infof("Received Envoy-Ext-Auth-Check to URL: %s", httpRequest.RequestURI)
	w := telemetry.NewInMemResponseWriter()
	(*proxy.compiler).ServeHTTP(w, httpRequest)

	var statusAllow bool
	switch w.StatusCode() {
	case http.StatusOK:
		statusAllow = true
	case http.StatusForbidden:
		statusAllow = false
	default:
		return false, errors.Wrap(errors.New(w.Body()), "EnvoyProxy: Error during request compilation")
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		decision := "DENY"
		if statusAllow {
			decision = "ALLOW"
		}
		logging.LogForComponent("envoyExtAuthzGrpcServer").WithFields(log.Fields{
			"dry-run":  proxy.grpcConfig.DryRun,
			"decision": decision,
		}).Debug("Returning policy decision.")
	}

	// If dry-run mode, override the status code to unconditionally allow the request
	// DecisionLogging should reflect what "would" have happened
	if proxy.grpcConfig.DryRun && !statusAllow {
		statusAllow = true
	}

	return statusAllow, nil
}
