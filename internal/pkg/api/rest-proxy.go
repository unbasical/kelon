package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/unbasical/kelon/pkg/constants"
	"github.com/unbasical/kelon/pkg/constants/logging"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/api"
)

type restProxy struct {
	pathPrefix string
	port       uint32
	configured bool
	appConf    *configs.AppConfig
	config     *api.ClientProxyConfig
	router     *mux.Router
	server     *http.Server

	metricsHandler http.Handler
}

// Implements api.ClientProxy by providing OPA's Data-REST-API.
func NewRestProxy(pathPrefix string, port uint32) api.ClientProxy {
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
func (proxy *restProxy) Configure(_ context.Context, appConf *configs.AppConfig, serverConf *api.ClientProxyConfig) error {
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

	ctx := context.Background()

	endpointData := proxy.pathPrefix + constants.EndpointSuffixData
	endpointPolicies := proxy.pathPrefix + constants.EndpointSuffixPolicies

	// Endpoints to validate queries
	proxy.router.PathPrefix(endpointData).Handler(proxy.applyHandlerMiddleware(ctx, endpointData, proxy.handleV1DataGet, withHeaderExtraction(true))).Methods("GET")
	proxy.router.PathPrefix(endpointData).Handler(proxy.applyHandlerMiddleware(ctx, endpointData, proxy.handleV1DataPost, withHeaderExtraction(true))).Methods("POST")

	// Endpoints to update policies and data
	proxy.router.PathPrefix(endpointData).Handler(proxy.applyHandlerMiddleware(ctx, endpointData, proxy.handleV1DataPut)).Methods("PUT")
	proxy.router.PathPrefix(endpointData).Handler(proxy.applyHandlerMiddleware(ctx, endpointData, proxy.handleV1DataPatch)).Methods("PATCH")
	proxy.router.PathPrefix(endpointData).Handler(proxy.applyHandlerMiddleware(ctx, endpointData, proxy.handleV1DataDelete)).Methods("DELETE")
	proxy.router.PathPrefix(endpointPolicies).Handler(proxy.applyHandlerMiddleware(ctx, endpointPolicies, proxy.handleV1PolicyPut)).Methods("PUT")
	proxy.router.PathPrefix(endpointPolicies).Handler(proxy.applyHandlerMiddleware(ctx, endpointPolicies, proxy.handleV1PolicyDelete)).Methods("DELETE")
	if proxy.metricsHandler != nil {
		logging.LogForComponent("restProxy").Infof("Registered %s endpoint", constants.EndpointMetrics)
		proxy.router.PathPrefix(constants.EndpointMetrics).Handler(proxy.metricsHandler)
	}
	proxy.router.PathPrefix(constants.EndpointHealth).Methods("GET").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("{\"status\": \"healthy\"}"))
	})

	proxy.server = &http.Server{
		Handler:           proxy.router,
		Addr:              fmt.Sprintf(":%d", proxy.port),
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 0,
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
