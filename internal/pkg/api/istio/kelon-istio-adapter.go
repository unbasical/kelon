// nolint:lll
// Generates the kelonistio adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.
//go:generate $REPO_ROOT/bin/mixer_codegen.sh -a mixer/adapter/kelon/config/config.proto -x "-s=false -n kelonistioadapter -t authorization"

package istio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"

	"istio.io/istio/mixer/pkg/status"

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

	MutualTLSConfig struct {
		CredentialFile  string
		PrivateKeyFile  string
		CertificateFile string
	}

	// Adapter supports metric template.
	Adapter struct {
		configured bool
		appConf    *configs.AppConfig
		config     *api.ClientProxyConfig
		compiler   *opa.PolicyCompiler
		listener   net.Listener
		server     *grpc.Server
	}
)

// Mapped type of a configured kelon-istio property
type PropertyType string

const (
	// Set configured property as request header
	PropHeader PropertyType = "header"
)

// Mappings for properties (They should be loaded from external config later on)
//nolint:gochecknoglobals
var propertyTypeMappings map[string]PropertyType = map[string]PropertyType{
	"authorization": PropHeader,
}

var _ authorization.HandleAuthorizationServiceServer = &Adapter{}

// ==============================================================
// Implement interface `ClientProxy` from api
// ==============================================================

// Implement Configure of pkg.api.ClientProxy
func (adapter *Adapter) Configure(appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
	// Exit if already configured
	if adapter.configured {
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
	adapter.appConf = appConf
	adapter.config = serverConf
	adapter.compiler = &compiler
	adapter.configured = true
	log.Infoln("Configured IstioProxy")
	return nil
}

// Implement Start of pkg.api.ClientProxy
func (adapter *Adapter) Start() error {
	if !adapter.configured {
		return errors.New("IstioProxy was not configured! Please call Configure(). ")
	}

	log.Infof("IstioProxy listening on: %s", adapter.Addr())
	adapter.server = grpc.NewServer()
	authorization.RegisterHandleAuthorizationServiceServer(adapter.server, adapter)

	go func() {
		shutdown := make(chan error, 1)
		adapter.Run(shutdown)
		if err := <-shutdown; err != nil {
			log.Fatalf("IstioProxy stopped unexpected: %s", err.Error())
		}
	}()

	return nil
}

// Implement Stop of pkg.api.ClientProxy
func (adapter *Adapter) Stop(deadline time.Duration) error {
	if adapter.server == nil {
		return errors.New("IstioProxy has not bin started yet")
	}

	return adapter.Close()
}

// ==============================================================
// Implement istio.adapter's `HandleAuthorizationServiceServer`
// ==============================================================

// Handle Authorization
func (adapter *Adapter) HandleAuthorization(ctx context.Context, req *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	// Write incoming parameters into request body
	action := req.Instance.Action
	log.Infof("IstioProxy: Handling incoming request: %s %s", action.Method, action.Path)

	httpRequest, err := http.NewRequest("POST", "/v1/data", strings.NewReader(fmt.Sprintf("{\"input\": {\"method\": \"%s\", \"path\": \"%s\"}}", action.Method, action.Path)))
	if err != nil {
		return nil, errors.Wrap(err, "IstioProxy: error while creating fake http request")
	}

	// Set property values
	if action.Properties != nil {
		for propertyKey, propertyValue := range action.Properties {
			mapping, exists := propertyTypeMappings[propertyKey]
			if !exists {
				return nil, errors.Errorf("IstioProxy: Incoming request had property [action.properties.%s] which was not mapped via configuration!", propertyKey)
			}
			switch mapping {
			case PropHeader:
				httpRequest.Header.Set(propertyKey, propertyValue.String())
			}
		}
	}

	decision, err := (*adapter.compiler).Process(httpRequest)
	if err != nil {
		return nil, errors.Wrap(err, "IstioProxy: Error during request compilation")
	}
	if !decision {
		return &v1beta1.CheckResult{
			Status: status.WithPermissionDenied("Kelon: request was rejected"),
		}, nil
	}

	// Accepted
	return &v1beta1.CheckResult{Status: status.OK}, nil
}

// ==============================================================
// Implement istio.adapter's `Handler`
// ==============================================================

// Addr returns the listening address of the server
func (adapter *Adapter) Addr() string {
	return adapter.listener.Addr().String()
}

// Run starts the server run
func (adapter *Adapter) Run(shutdown chan error) {
	shutdown <- adapter.server.Serve(adapter.listener)
}

// Close gracefully shuts down the server; used for testing
func (adapter *Adapter) Close() error {
	if adapter.server != nil {
		adapter.server.GracefulStop()
	}

	if adapter.listener != nil {
		_ = adapter.listener.Close()
	}

	return nil
}

// NewKelonIstioAdapter creates a new IBP adapter that listens at provided port.
func NewKelonIstioAdapter(port uint32, creds *MutualTLSConfig) (api.ClientProxy, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("unable to listen on socket: %v", err)
	}
	s := &Adapter{
		listener: listener,
	}

	if creds != nil {
		so, err := getServerTLSOption(creds.CredentialFile, creds.PrivateKeyFile, creds.CertificateFile)
		if err != nil {
			return nil, err
		}
		log.Infof("IstioProxy: configured mutual TLS with credentials: %q, privateKey: %q, certificate: %q", creds.CredentialFile, creds.PrivateKeyFile, creds.CertificateFile)
		s.server = grpc.NewServer(so)
	} else {
		s.server = grpc.NewServer()
	}
	return s, nil
}

func getServerTLSOption(credential, privateKey, caCertificate string) (grpc.ServerOption, error) {
	certificate, err := tls.LoadX509KeyPair(
		credential,
		privateKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load key cert pair")
	}
	certPool := x509.NewCertPool()
	bs, err := ioutil.ReadFile(caCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to read client ca cert: %s", err)
	}

	ok := certPool.AppendCertsFromPEM(bs)
	if !ok {
		return nil, fmt.Errorf("failed to append client certs")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    certPool,
	}
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	return grpc.Creds(credentials.NewTLS(tlsConfig)), nil
}
