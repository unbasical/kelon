package util

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
type ContextKey string // can be unexported

// ContextKeyRequestID is the ContextKey for RequestID
const ContextKeyRequestID ContextKey = "requestUID" // can be unexported

// AttachRequestID will attach a brand new request ID to a http request
func AssignRequestUID(req *http.Request) *http.Request {
	reqID := uuid.New()
	ctx := req.Context()
	return req.WithContext(context.WithValue(ctx, ContextKeyRequestID, reqID.String()))
}

// GetRequestID will get reqID from a http request and return it as a string
func GetRequestUID(req *http.Request) string {
	reqID := req.Context().Value(ContextKeyRequestID)
	if ret, ok := reqID.(string); ok {
		return ret
	}
	return ""
}
