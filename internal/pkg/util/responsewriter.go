package util

import (
	"net/http"
)

type InMemResponseWriter struct {
	body       []byte
	statusCode int
	header     http.Header
}

func NewInMemResponseWriter() *InMemResponseWriter {
	return &InMemResponseWriter{
		header: http.Header{},
	}
}

func (w *InMemResponseWriter) Header() http.Header {
	return w.header
}

func (w *InMemResponseWriter) Body() string {
	return string(w.body)
}

func (w *InMemResponseWriter) StatusCode() int {
	return w.statusCode
}

func (w *InMemResponseWriter) Write(b []byte) (int, error) {
	w.body = b
	// implement it as per your requirement
	return 0, nil
}

func (w *InMemResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}
