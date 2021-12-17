package envoy

import (
	authv2 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
)

type ExternalRequest interface {
	GetMethod() string
	GetHeaders() map[string]string
	GetPath() string
	GetQuery() string
	GetBody() string
	GetScheme() string
}

type ExternalRequestV2 struct {
	request *authv2.AttributeContext_HttpRequest
}

func (r ExternalRequestV2) GetMethod() string {
	return r.request.GetMethod()
}

func (r ExternalRequestV2) GetHeaders() map[string]string {
	return r.request.GetHeaders()
}

func (r ExternalRequestV2) GetPath() string {
	return r.request.GetPath()
}

func (r ExternalRequestV2) GetQuery() string {
	return r.request.GetQuery()
}

func (r ExternalRequestV2) GetBody() string {
	return r.request.GetBody()
}

func (r ExternalRequestV2) GetScheme() string {
	return r.request.GetScheme()
}

type ExternalRequestV3 struct {
	request *authv3.AttributeContext_HttpRequest
}

func (r ExternalRequestV3) GetMethod() string {
	return r.request.GetMethod()
}

func (r ExternalRequestV3) GetHeaders() map[string]string {
	return r.request.GetHeaders()
}

func (r ExternalRequestV3) GetPath() string {
	return r.request.GetPath()
}

func (r ExternalRequestV3) GetQuery() string {
	return r.request.GetQuery()
}

func (r ExternalRequestV3) GetBody() string {
	return r.request.GetBody()
}

func (r ExternalRequestV3) GetScheme() string {
	return r.request.GetScheme()
}
