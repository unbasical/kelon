package envoy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	extauthz "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/api"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/telemetry"
	"google.golang.org/genproto/googleapis/rpc/code"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Config represents the plugin configuration.
type Config struct {
	Port                   uint32 `json:"port"`
	DryRun                 bool   `json:"dry-run"`
	EnableReflection       bool   `json:"enable-reflection"`
	AccessDecisionLogLevel string
}

type envoyExtAuthzGrpcServer struct {
	cfg                 Config
	appConf             *configs.AppConfig
	server              *grpc.Server
	compiler            *http.Handler
	preparedQueryDoOnce *sync.Once
}

type envoyProxy struct {
	configured bool
	appConf    *configs.AppConfig
	config     *api.ClientProxyConfig
	envoy      *envoyExtAuthzGrpcServer
}

// Implements api.ClientProxy by providing OPA's Data-REST-API.
func NewEnvoyProxy(config Config) api.ClientProxy {
	if config.Port == 0 {
		logging.LogForComponent("Config").Warnln("EnvoyProxy was initialized with default properties! You may have missed some arguments when creating it!")
		config.Port = 9191
		config.DryRun = false
		config.EnableReflection = true
	}

	return &envoyProxy{
		configured: false,
		appConf:    nil,
		config:     nil,
		envoy: &envoyExtAuthzGrpcServer{
			cfg:                 config,
			appConf:             nil,
			server:              nil,
			compiler:            nil,
			preparedQueryDoOnce: nil,
		},
	}
}

// See Configure() of api.ClientProxy
func (proxy *envoyProxy) Configure(ctx context.Context, appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
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
	handler := appConf.MetricsProvider.WrapHTTPHandler(ctx, compiler)

	// Assign variables
	proxy.envoy.compiler = &handler
	proxy.appConf = appConf
	proxy.config = serverConf
	proxy.configured = true
	proxy.envoy.appConf = appConf
	logging.LogForComponent("envoyProxy").Infoln("Configured")
	return nil
}

// See Start() of api.ClientProxy
func (proxy *envoyProxy) Start() error {
	if !proxy.configured {
		return errors.Errorf("EnvoyProxy was not configured! Please call Configure(). ")
	}

	// Init grpc server
	interceptor := proxy.makeServerInterceptor()
	proxy.envoy.server = grpc.NewServer(interceptor)
	// Register Authorization Server
	extauthz.RegisterAuthorizationServer(proxy.envoy.server, proxy.envoy)

	// Register reflection service on gRPC server
	if proxy.envoy.cfg.EnableReflection {
		reflection.Register(proxy.envoy.server)
	}

	logging.LogForComponent("envoyProxy").Infof("Starting envoy grpc-server at: http://0.0.0.0:%d", proxy.envoy.cfg.Port)
	return proxy.envoy.Start(context.Background())
}

// See Stop() of api.ClientProxy
func (proxy *envoyProxy) Stop(deadline time.Duration) error {
	if proxy.envoy.server == nil {
		return errors.Errorf("EnvoyProxy has not bin started yet")
	}

	logging.LogForComponent("envoyProxy").Infof("Stopping envoy grpc-server at: http://0.0.0.0:%d", proxy.envoy.cfg.Port)
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	proxy.envoy.Stop(ctx)
	return nil
}

// Start the underlying grpc-server
func (p *envoyExtAuthzGrpcServer) Start(ctx context.Context) error {
	go p.listen()
	return nil
}

// Stop the underlying grpc-server
func (p *envoyExtAuthzGrpcServer) Stop(ctx context.Context) {
	p.server.Stop()
}

// Reconfigure the underlying grpc-server (Unused! Just to conform with the interface)
func (p *envoyExtAuthzGrpcServer) Reconfigure(ctx context.Context, config interface{}) {
}

func (p *envoyExtAuthzGrpcServer) listen() {
	// The listener is closed automatically by Serve when it returns.
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", p.cfg.Port))
	if err != nil {
		logging.LogForComponent("envoyExtAuthzGrpcServer").WithError(err).Fatal("Unable to create listener.")
	}

	log.WithFields(log.Fields{
		"port":                 p.cfg.Port,
		"dry-run":              p.cfg.DryRun,
		"enable-reflection":    p.cfg.EnableReflection,
		logging.LabelComponent: "envoyExtAuthzGrpcServer",
	}).Info("Starting gRPC server.")

	if err := p.server.Serve(l); err != nil {
		logging.LogForComponent("envoyExtAuthzGrpcServer").WithError(err).Fatal("Listener failed.")
	}

	logging.LogForComponent("envoyExtAuthzGrpcServer").Info("Listener exited.")
}

// Check a new incoming request
func (p *envoyExtAuthzGrpcServer) Check(ctx context.Context, req *extauthz.CheckRequest) (*extauthz.CheckResponse, error) {
	// Rebuild http request
	r := req.GetAttributes().GetRequest().GetHttp()
	path := r.GetPath()
	if r.Query != "" {
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

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", "http://envoy.ext.auth.proxy/v1/data", strings.NewReader(inputBody))
	if err != nil {
		return nil, errors.Wrap(err, "EnvoyProxy: Unable to reconstruct HTTP-Request")
	}

	// Set headers
	for headerKey, headerValue := range r.GetHeaders() {
		httpRequest.Header.Set(headerKey, headerValue)
	}

	logging.LogForComponent("envoyExtAuthzGrpcServer").Infof("Received Envoy-Ext-Auth-Check to URL: %s", httpRequest.RequestURI)

	w := telemetry.NewInMemResponseWriter()
	(*p.compiler).ServeHTTP(w, httpRequest)

	resp := &extauthz.CheckResponse{}
	switch w.StatusCode() {
	case http.StatusOK:
		resp.Status = &rpcstatus.Status{Code: int32(code.Code_OK)}
	case http.StatusForbidden:
		resp.Status = &rpcstatus.Status{Code: int32(code.Code_PERMISSION_DENIED)}
	default:
		proxyErr := errors.Wrap(errors.New(w.Body()), "EnvoyProxy: Error during request compilation")
		return nil, proxyErr
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		decision := "DENY"
		if w.StatusCode() == http.StatusOK {
			decision = "ALLOW"
		}
		logging.LogForComponent("envoyExtAuthzGrpcServer").WithFields(log.Fields{
			"dry-run":  p.cfg.DryRun,
			"decision": decision,
		}).WithError(err).Debug("Returning policy decision.")
	}

	// If dry-run mode, override the status code to unconditionally allow the request
	// DecisionLogging should reflect what "would" have happened
	if p.cfg.DryRun {
		if resp.Status.Code != int32(code.Code_OK) {
			resp.Status = &rpcstatus.Status{Code: int32(code.Code_OK)}
			resp.HttpResponse = &extauthz.CheckResponse_OkResponse{
				OkResponse: &extauthz.OkHttpResponse{},
			}
		}
	}

	return resp, nil
}

func (proxy *envoyProxy) makeServerInterceptor() grpc.ServerOption {
	var interceptors []grpc.UnaryServerInterceptor

	if proxy.appConf.MetricsProvider != nil {
		interceptors = append(interceptors, proxy.appConf.MetricsProvider.GetGrpcServerInterceptor())
	}

	if proxy.appConf.TraceProvider != nil {
		interceptors = append(interceptors, proxy.appConf.TraceProvider.GetGrpcServerInterceptor())
	}

	return grpc.ChainUnaryInterceptor(interceptors...)
}
