package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/unbasical/kelon/pkg/constants/logging"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/api"
)

type restProxy struct {
	pathPrefix string
	port       int32
	configured bool
	appConf    *configs.AppConfig
	config     *api.ClientProxyConfig
	router     *mux.Router
	server     *http.Server

	metricsHandler    http.Handler
	metricsMiddleware func(handler http.Handler) http.Handler
}

// Implements api.ClientProxy by providing OPA's Data-REST-API.
func NewRestProxy(pathPrefix string, port int32) api.ClientProxy {
	return &restProxy{
		pathPrefix: pathPrefix,
		port:       port,
		configured: false,
		appConf:    nil,
		config:     nil,
		router:     mux.NewRouter(),
	}
}

// See Configure() of api.ClientProxy
func (proxy *restProxy) Configure(ctx context.Context, appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
	// Exit if already configured
	if proxy.configured {
		return nil
	}

	// Configure subcomponents
	if serverConf.Compiler == nil {
		return errors.Errorf("RestProxy: Compiler not configured! ")
	}
	compiler := *serverConf.Compiler
	if err := compiler.Configure(appConf, &serverConf.PolicyCompilerConfig); err != nil {
		return err
	}

	// Configure telemetry (if set)
	if appConf.MetricsProvider != nil {
		if metricsHandler, handlerErr := appConf.MetricsProvider.GetHTTPMetricsHandler(); handlerErr == nil {
			proxy.metricsHandler = metricsHandler
		}

		telemetryMiddleware, middErr := appConf.MetricsProvider.GetHTTPMiddleware(ctx)
		if middErr != nil {
			return errors.Wrap(middErr, "RestProxy was configured with MetricsProvider that does not implement 'GetHTTPMiddleware()' correctly.")
		}
		proxy.metricsMiddleware = telemetryMiddleware
	}

	// Assign variables
	proxy.appConf = appConf
	proxy.config = serverConf
	proxy.configured = true
	logging.LogForComponent("restProxy").Infoln("Configured RestProxy")
	return nil
}

// See Start() of api.ClientProxy
func (proxy *restProxy) Start() error {
	if !proxy.configured {
		err := errors.Errorf("RestProxy was not configured! Please call Configure(). ")
		return err
	}

	// Endpoints to validate queries
	proxy.router.PathPrefix(proxy.pathPrefix + "/data").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1DataGet)).Methods("GET")
	proxy.router.PathPrefix(proxy.pathPrefix + "/data").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1DataPost)).Methods("POST")

	// Endpoints to update policies and data
	proxy.router.PathPrefix(proxy.pathPrefix + "/data").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1DataPut)).Methods("PUT")
	proxy.router.PathPrefix(proxy.pathPrefix + "/data").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1DataPatch)).Methods("PATCH")
	proxy.router.PathPrefix(proxy.pathPrefix + "/data").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1DataDelete)).Methods("DELETE")
	proxy.router.PathPrefix(proxy.pathPrefix + "/policies").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1PolicyPut)).Methods("PUT")
	proxy.router.PathPrefix(proxy.pathPrefix + "/policies").Handler(proxy.applyHandlerMiddlewareIfSet(proxy.handleV1PolicyDelete)).Methods("DELETE")
	if proxy.metricsHandler != nil {
		logging.LogForComponent("restProxy").Infoln("Registered /metrics endpoint")
		proxy.router.PathPrefix("/metrics").Handler(proxy.metricsHandler)
	}
	proxy.router.PathPrefix("/health").Methods("GET").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("{\"status\": \"healthy\"}"))
	})

	proxy.server = &http.Server{
		Handler:      proxy.router,
		Addr:         fmt.Sprintf(":%d", proxy.port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		logging.LogForComponent("restProxy").Infof("Starting server at: http://0.0.0.0:%d%s", proxy.port, proxy.pathPrefix)
		if err := proxy.server.ListenAndServe(); err != nil {
			logging.LogForComponent("restProxy").Warn(err)
		}
	}()
	return nil
}

func (proxy *restProxy) applyHandlerMiddlewareIfSet(handlerFunc func(http.ResponseWriter, *http.Request)) http.Handler {
	if proxy.metricsMiddleware != nil {
		return proxy.metricsMiddleware(http.HandlerFunc(handlerFunc))
	}
	return http.HandlerFunc(handlerFunc)
}

// See Stop() of api.ClientProxy
func (proxy *restProxy) Stop(deadline time.Duration) error {
	if proxy.server == nil {
		return errors.Errorf("RestProxy has not bin started yet")
	}

	logging.LogForComponent("restProxy").Infof("Stopping server at: http://localhost:%d%s", proxy.port, proxy.pathPrefix)

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	onShutdown := make(chan struct{})
	defer cancel()

	proxy.server.RegisterOnShutdown(func() {
		onShutdown <- struct{}{}
	})
	proxy.server.SetKeepAlivesEnabled(false)
	if err := proxy.server.Shutdown(ctx); err != nil {
		logging.LogForComponent("restProxy").WithError(err).Error("Error while shutting down server")
		return errors.Wrap(err, "Error while shutting down server")
	}

	select {
	case <-onShutdown:
		logging.LogForComponent("restProxy").Info("Server shutdown completed")
		return nil
	case <-ctx.Done():
		return errors.Errorf("Server failed to shutdown before timeout!")
	}
}
