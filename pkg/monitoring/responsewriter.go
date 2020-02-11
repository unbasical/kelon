package monitoring

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

type PassThroughResponseWriter struct {
	body       []byte
	statusCode int
	header     http.Header
	writer     http.ResponseWriter
}

func NewPassThroughResponseWriter(w http.ResponseWriter) *PassThroughResponseWriter {
	return &PassThroughResponseWriter{
		header: http.Header{},
		writer: w,
	}
}

func (w *PassThroughResponseWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *PassThroughResponseWriter) Body() string {
	return string(w.body)
}

func (w *PassThroughResponseWriter) StatusCode() int {
	return w.statusCode
}

func (w *PassThroughResponseWriter) Write(b []byte) (int, error) {
	w.body = b
	return w.writer.Write(b)
}

func (w *PassThroughResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.writer.WriteHeader(statusCode)
}
