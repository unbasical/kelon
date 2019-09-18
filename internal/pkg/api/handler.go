package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

func (proxy restProxy) handleGet(w http.ResponseWriter, r *http.Request) {
	// Map query parameter "input" to request body
	body := ""
	if keys, ok := r.URL.Query()["input"]; ok && len(keys) == 1 {
		// Assign body
		body = keys[0]
	} else {
		log.Println("RestProxy: Received GET request without input: " + r.URL.String())
	}

	if trans, err := http.NewRequest("POST", r.URL.String(), strings.NewReader(body)); err == nil {
		// Handle request like post
		proxy.handlePost(w, trans)
	} else {
		log.Fatal("RestProxy: Unable to map GET request to POST: ", err.Error())
	}
}

func (proxy restProxy) handlePost(w http.ResponseWriter, r *http.Request) {
	// Compile
	compiler := *proxy.config.Compiler
	if decision, err := compiler.Process(r); err == nil {
		// Map response
		var response string
		switch decision {
		case true:
			response = "ALLOWED"
		case false:
			response = "DENIED"
		}

		// Respond to calling client
		if _, err := fmt.Fprint(w, response); err != nil {
			log.Fatal("Unable to respond to HTTP request")
		}
	} else {
		log.Fatal("RestProxy: Unable to compile request: ", err.Error())
	}
}
