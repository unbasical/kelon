package api

import (
	"context"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type RestProxy struct {
	PathPrefix string
	Port       int32
	Config     *configs.AppConfig
	router     *mux.Router
}

func NewRestProxy(pathPrefix string, port int32, conf *configs.AppConfig) *RestProxy {
	proxy := new(RestProxy)

	proxy.PathPrefix = pathPrefix
	proxy.Port = port
	proxy.Config = conf
	proxy.router = mux.NewRouter()

	return proxy
}

func (proxy RestProxy) Start() {
	// Create Server and Route Handlers
	proxy.router.PathPrefix(proxy.PathPrefix).HandlerFunc(proxy.handleGet).Methods("GET")
	proxy.router.PathPrefix(proxy.PathPrefix).HandlerFunc(proxy.handlePost).Methods("POST")

	server := &http.Server{
		Handler:      proxy.router,
		Addr:         fmt.Sprintf(":%d", proxy.Port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		log.Println("Starting Server")
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for shutdown
	gracefulShutdown(server)
}

func gracefulShutdown(server *http.Server) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalln("Error while shutting down server: ", err.Error())
	}

	log.Println("Shutting down...")
	os.Exit(0)
}
