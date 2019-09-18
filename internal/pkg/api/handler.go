package api

import (
	"fmt"
	"log"
	"net/http"
)

func (proxy restProxy) handleGet(w http.ResponseWriter, r *http.Request) {
	if _, err := fmt.Fprintf(w, "Hi there, you executed a GET request to OPA's Data-API via kelon: %s!", r.URL.Path[1:]); err != nil {
		log.Fatal("Unable to respond to HTTP request")
	}
}

func (proxy restProxy) handlePost(w http.ResponseWriter, r *http.Request) {
	if _, err := fmt.Fprintf(w, "Hi there, you executed a POST request to OPA's Data-API via kelon: %s!", r.URL.Path[1:]); err != nil {
		log.Fatal("Unable to respond to HTTP request")
	}
}
