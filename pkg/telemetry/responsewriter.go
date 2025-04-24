package telemetry

import (
	"net/http"
)

// PassThroughResponseWriter wraps a http.ResponseWriter but the status code and body can be accessed.
// This is useful for e.g. logging and tracing
type PassThroughResponseWriter struct {
	body       []byte
	statusCode int
	header     http.Header
	writer     http.ResponseWriter
}

// NewPassThroughResponseWriter wraps the given http.ResponseWriter
func NewPassThroughResponseWriter(w http.ResponseWriter) *PassThroughResponseWriter {
	return &PassThroughResponseWriter{
		header: http.Header{},
		writer: w,
	}
}

// Header returns the response http.Header
func (w *PassThroughResponseWriter) Header() http.Header {
	return w.writer.Header()
}

// Body returns the response body
func (w *PassThroughResponseWriter) Body() []byte { return w.body }

// StatusCode returns the response status code
func (w *PassThroughResponseWriter) StatusCode() int {
	return w.statusCode
}

// Write records the data and passes the buffer to the underlying http.ResponseWriter
func (w *PassThroughResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.writer.Write(b)
}

// WriteHeader records the statusCode and passes it on to the underlying http.ResponseWriter
func (w *PassThroughResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.writer.WriteHeader(statusCode)
}
