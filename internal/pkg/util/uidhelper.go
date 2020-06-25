package util

import (
	"context"
	"net/http"

	"github.com/Foundato/kelon/pkg/constants"

	"github.com/google/uuid"
)

// AttachRequestID will attach a brand new request ID to a http request
func AssignRequestUID(req *http.Request) *http.Request {
	reqID := uuid.New()
	ctx := req.Context()
	return req.WithContext(context.WithValue(ctx, constants.ContextKeyRequestID, reqID.String()))
}

// GetRequestID will get reqID from a http request and return it as a string
func GetRequestUID(req *http.Request) string {
	reqID := req.Context().Value(constants.ContextKeyRequestID)
	if ret, ok := reqID.(string); ok {
		return ret
	}
	return ""
}
