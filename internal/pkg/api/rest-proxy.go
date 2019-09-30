package api

import (
	"context"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type restProxy struct {
	pathPrefix string
	port       int32
	configured bool
	appConf    *configs.AppConfig
	config     *ClientProxyConfig
	router     *mux.Router
	server     *http.Server
}

func NewRestProxy(pathPrefix string, port int32) ClientProxy {
	return &restProxy{
		pathPrefix: pathPrefix,
		port:       port,
		configured: false,
		appConf:    nil,
		config:     nil,
		router:     mux.NewRouter(),
	}
}

func (proxy *restProxy) Configure(appConf *configs.AppConfig, serverConf *ClientProxyConfig) error {
	// Configure subcomponents
	if serverConf.Compiler == nil {
		return errors.New("RestProxy: Compiler not configured! ")
	}
	compiler := *serverConf.Compiler
	if err := compiler.Configure(appConf, &serverConf.PolicyCompilerConfig); err != nil {
		return err
	}

	// Assign variables
	proxy.appConf = appConf
	proxy.config = serverConf
	proxy.configured = true
	log.Infoln("Configured RestProxy")
	return nil
}

func (proxy *restProxy) Start() error {
	if !proxy.configured {
		return errors.New("RestProxy was not configured! Please call Configure(). ")
	}

	// Create Server and Route Handlers
	proxy.router.PathPrefix(proxy.pathPrefix).HandlerFunc(proxy.handleGet).Methods("GET")
	proxy.router.PathPrefix(proxy.pathPrefix).HandlerFunc(proxy.handlePost).Methods("POST")

	proxy.server = &http.Server{
		Handler:      proxy.router,
		Addr:         fmt.Sprintf(":%d", proxy.port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		log.Infof("Starting server at: http://localhost:%d%s\n", proxy.port, proxy.pathPrefix)
		if err := proxy.server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (proxy *restProxy) Stop(deadline time.Duration) error {
	log.Infof("Stopping server at: http://localhost:%d%s\n", proxy.port, proxy.pathPrefix)
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	if err := proxy.server.Shutdown(ctx); err != nil {
		return errors.Wrap(err, "Error while shutting down server")
	}
	return nil
}
