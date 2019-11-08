package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/Foundato/kelon/pkg/opa"
	ext_authz "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/rpc/code"
	rpc_status "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Config represents the plugin configuration.
type EnvoyConfig struct {
	Addr             string `json:"addr"`
	DryRun           bool   `json:"dry-run"`
	EnableReflection bool   `json:"enable-reflection"`
}

type envoyExtAuthzGrpcServer struct {
	cfg                 EnvoyConfig
	server              *grpc.Server
	compiler            *opa.PolicyCompiler
	preparedQueryDoOnce *sync.Once
}

type envoyProxy struct {
	configured bool
	appConf    *configs.AppConfig
	config     *api.ClientProxyConfig
	envoy      *envoyExtAuthzGrpcServer
}

// Implements api.ClientProxy by providing OPA's Data-REST-API.
func NewEnvoyProxy(config EnvoyConfig) api.ClientProxy {
	if config.Addr == "" {
		log.Warnln("EnvoyProxy was initialized with default properties! You may have missed some arguments when creating it!")
		config.Addr = ":9191"
		config.DryRun = false
		config.EnableReflection = false
	}

	return &envoyProxy{
		configured: false,
		appConf:    nil,
		config:     nil,
		envoy: &envoyExtAuthzGrpcServer{
			cfg:                 config,
			server:              nil,
			compiler:            nil,
			preparedQueryDoOnce: nil,
		},
	}
}

// See Configure() of api.ClientProxy
func (proxy *envoyProxy) Configure(appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
	// Configure subcomponents
	if serverConf.Compiler == nil {
		return errors.New("EnvoyProxy: Compiler not configured! ")
	}
	compiler := *serverConf.Compiler
	if err := compiler.Configure(appConf, &serverConf.PolicyCompilerConfig); err != nil {
		return err
	}

	// Assign variables
	proxy.appConf = appConf
	proxy.config = serverConf
	proxy.envoy.compiler = serverConf.Compiler
	proxy.configured = true
	log.Infoln("Configured EnvoyProxy")
	return nil
}

// See Start() of api.ClientProxy
func (proxy *envoyProxy) Start() error {
	if !proxy.configured {
		return errors.New("RestProxy was not configured! Please call Configure(). ")
	}

	// Init grpc server
	proxy.envoy.server = grpc.NewServer()
	// Register Authorization Server
	ext_authz.RegisterAuthorizationServer(proxy.envoy.server, proxy.envoy)

	// Register reflection service on gRPC server
	if proxy.envoy.cfg.EnableReflection {
		reflection.Register(proxy.envoy.server)
	}

	return proxy.envoy.Start(context.Background())
}

// See Stop() of api.ClientProxy
func (proxy *envoyProxy) Stop(deadline time.Duration) error {
	if proxy.envoy.server == nil {
		return errors.New("RestProxy has not bin started yet")
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	proxy.envoy.Stop(ctx)
	return nil
}

func (p *envoyExtAuthzGrpcServer) Start(ctx context.Context) error {
	go p.listen()
	return nil
}

func (p *envoyExtAuthzGrpcServer) Stop(ctx context.Context) {
	p.server.Stop()
}

func (p *envoyExtAuthzGrpcServer) Reconfigure(ctx context.Context, config interface{}) {
}

func (p *envoyExtAuthzGrpcServer) listen() {
	// The listener is closed automatically by Serve when it returns.
	l, err := net.Listen("tcp", p.cfg.Addr)
	if err != nil {
		log.WithField("err", err).Fatal("EnvoyProxy: Unable to create listener.")
	}

	log.WithFields(log.Fields{
		"addr":              p.cfg.Addr,
		"dry-run":           p.cfg.DryRun,
		"enable-reflection": p.cfg.EnableReflection,
	}).Info("EnvoyProxy: Starting gRPC server.")

	if err := p.server.Serve(l); err != nil {
		log.WithField("err", err).Fatal("EnvoyProxy: Listener failed.")
	}

	log.Info("EnvoyProxy: Listener exited.")
}

func (p *envoyExtAuthzGrpcServer) Check(ctx context.Context, req *ext_authz.CheckRequest) (*ext_authz.CheckResponse, error) {
	// Rebuild http request
	r := req.GetAttributes().GetRequest().GetHttp()
	httpRequest, err := http.NewRequest(r.GetMethod(), fmt.Sprintf("%s%s?%s", r.GetHost(), r.GetPath(), r.GetQuery()), strings.NewReader(r.GetBody()))
	if err != nil {
		return nil, errors.Wrap(err, "EnvoyProxy: Unable to reconstruct HTTP-Request")
	}
	// Set headers
	for headerKey, headerValue := range r.GetHeaders() {
		httpRequest.Header.Set(headerKey, headerValue)
	}

	decision, err := (*p.compiler).Process(httpRequest)
	if err != nil {
		return nil, errors.Wrap(err, "EnvoyProxy: Error during request compilation")
	}

	resp := &ext_authz.CheckResponse{}
	resp.Status = &rpc_status.Status{Code: int32(code.Code_PERMISSION_DENIED)}
	if decision {
		resp.Status = &rpc_status.Status{Code: int32(code.Code_OK)}
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		log.WithFields(log.Fields{
			"dry-run":  p.cfg.DryRun,
			"decision": decision,
			"err":      err,
		}).Debug("Returning policy decision.")
	}

	// If dry-run mode, override the Status code to unconditionally Allow the request
	// DecisionLogging should reflect what "would" have happened
	if p.cfg.DryRun {
		if resp.Status.Code != int32(code.Code_OK) {
			resp.Status = &rpc_status.Status{Code: int32(code.Code_OK)}
			resp.HttpResponse = &ext_authz.CheckResponse_OkResponse{
				OkResponse: &ext_authz.OkHttpResponse{},
			}
		}
	}

	return resp, nil
}
