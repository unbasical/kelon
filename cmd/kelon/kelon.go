package main

import (
	"context"
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/Foundato/kelon/internal/pkg/watcher"
	"github.com/gorilla/mux"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	app = kingpin.New("kelon", "Kelon policy enforcer.")
	// Commands
	start = app.Command("start", "Start kelon in production mode.")
	debug = app.Command("debug", "Enable debug mode.")
	// Flags
	datastorePath = app.Flag("datastore-conf", "Path to the datastore configuration yaml.").Short('d').Default("./datastore.yml").Envar("DATASTORE_CONF").ExistingFile()
	apiPath       = app.Flag("api-conf", "Path to the api configuration yaml.").Short('a').Default("./api.yml").Envar("API_CONF").ExistingFile()
	proxyUrl      = app.Flag("proxy-url", "URL which is used to proxy OPA's Data-Api.").Default("/v1/data/authz/allow").Envar("PROXY_URL").URL()
	port          = app.Flag("port", "Port on which the proxy endpoint is served.").Short('p').Default("8181").Envar("PORT").Int32()
)

type AppConfig struct {
	debug bool
}

var appConf = AppConfig{}

func handler(w http.ResponseWriter, r *http.Request) {
	if _, err := fmt.Fprintf(w, "Hi there, kelon responding to %s!", r.URL.Path[1:]); err != nil {
		log.Fatal("Unable to respond to HTTP request")
	}
}

func main() {
	app.HelpFlag.Short('h')
	app.Version("0.1.0")

	// Parse args
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case start.FullCommand():
		appConf.debug = false
	case debug.FullCommand():
		appConf.debug = true
	}

	// Run app
	println("Kelon starting...")

	// Init config loader
	configLoader := configs.FileConfigLoader{
		DatastoreConfigPath: *datastorePath,
		ApiConfigPath:       *apiPath,
	}
	// Start app after config is present
	watcher.DefaultConfigWatcher{Loader: configLoader}.Watch(onConfigLoaded)
}

func onConfigLoaded(config *configs.Config, err error) {
	if err != nil {
		log.Fatalln("Unable to parse configuration: ", err.Error())
	}

	// Create Server and Route Handlers
	log.Printf("Serving OPA's Data-Api at: http://localhost:%d%s\n", *port, (*proxyUrl).String())
	r := mux.NewRouter()
	r.HandleFunc((*proxyUrl).String(), handler)

	server := &http.Server{
		Handler:      r,
		Addr:         fmt.Sprintf(":%d", *port),
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
