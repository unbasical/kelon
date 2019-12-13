// nolint:lll
// Generates the kelonistio adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.
//go:generate $REPO_ROOT/bin/mixer_codegen.sh -a mixer/adapter/kelon/config/config.proto -x "-s=false -n kelonistioadapter -t authorization"

package istio

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	google_rpc "istio.io/gogo-genproto/googleapis/google/rpc"

	"github.com/Foundato/kelon/pkg/opa"

	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/pkg/api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
)

type (
	// Server is basic server interface
	Server interface {
		Addr() string
		Close() error
		Run(shutdown chan error)
	}

	// KelonIstioAdapter supports metric template.
	KelonIstioAdapter struct {
		configured bool
		appConf    *configs.AppConfig
		config     *api.ClientProxyConfig
		compiler   *opa.PolicyCompiler
		listener   net.Listener
		server     *grpc.Server
	}
)

var _ authorization.HandleAuthorizationServiceServer = &KelonIstioAdapter{}

// ==============================================================
// Implement interface `ClientProxy` from api
// ==============================================================

// Implement Configure of pkg.api.ClientProxy
func (s *KelonIstioAdapter) Configure(appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
	// Exit if already configured
	if s.configured {
		return nil
	}

	// Configure subcomponents
	if serverConf.Compiler == nil {
		return errors.New("IstioProxy: Compiler not configured! ")
	}
	compiler := *serverConf.Compiler
	if err := compiler.Configure(appConf, &serverConf.PolicyCompilerConfig); err != nil {
		return err
	}

	// Assign variables
	s.appConf = appConf
	s.config = serverConf
	s.compiler = &compiler
	s.configured = true
	log.Infoln("Configured IstioProxy")
	return nil
}

// Implement Start of pkg.api.ClientProxy
func (s *KelonIstioAdapter) Start() error {
	if !s.configured {
		return errors.New("IstioProxy was not configured! Please call Configure(). ")
	}

	log.Infof("IstioProxy listening on: %s", s.Addr())
	s.server = grpc.NewServer()
	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)

	go func() {
		shutdown := make(chan error, 1)
		s.Run(shutdown)
		if err := <-shutdown; err != nil {
			log.Fatalf("IstioProxy stopped unexpected: %s", err.Error())
		}
	}()

	return nil
}

// Implement Stop of pkg.api.ClientProxy
func (s *KelonIstioAdapter) Stop(deadline time.Duration) error {
	if s.server == nil {
		return errors.New("IstioProxy has not bin started yet")
	}

	return s.Close()
}

// ==============================================================
// Implement istio.adapter's `HandleAuthorizationServiceServer`
// ==============================================================

// Handle Authorization
func (s *KelonIstioAdapter) HandleAuthorization(ctx context.Context, req *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	log.Infof("IstioProxy received request %v", *req)
	action := req.Instance.Action
	httpRequest, err := http.NewRequest(action.Method, action.Path, strings.NewReader(""))
	if err != nil {
		return nil, errors.Wrap(err, "IstioProxy: error while creating fake http request")
	}

	decision, err := (*s.compiler).Process(httpRequest)
	if err != nil {
		return nil, errors.Wrap(err, "EnvoyProxy: Error during request compilation")
	}
	log.Infof("Handle opa decision %+v", decision)

	return &v1beta1.CheckResult{
		Status: google_rpc.Status{
			Code:    10,
			Message: "Error",
			Details: nil,
		},
		ValidDuration: 0,
		ValidUseCount: 0,
	}, errors.New("This failed")
}

// ==============================================================
// Implement istio.adapter's `Handler`
// ==============================================================

// Addr returns the listening address of the server
func (s *KelonIstioAdapter) Addr() string {
	return s.listener.Addr().String()
}

// Run starts the server run
func (s *KelonIstioAdapter) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

// Close gracefully shuts down the server; used for testing
func (s *KelonIstioAdapter) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

// NewKelonIstioAdapter creates a new IBP adapter that listens at provided port.
func NewKelonIstioAdapter(port uint32) api.ClientProxy {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("unable to listen on socket: %v", err)
	}
	s := &KelonIstioAdapter{
		listener: listener,
	}
	return s
}
