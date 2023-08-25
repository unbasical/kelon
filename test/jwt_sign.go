package test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"
	"net/url"
	"reflect"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/authn"
	"gopkg.in/square/go-jose.v2"
)

func key(store authn.KeyStore, src *url.URL) (*jose.JSONWebKey, string, error) {
	keys, err := store.ResolveKeySet(src)
	if err != nil {
		return nil, "", err
	}

	var pk jose.JSONWebKey
	var kid string
	for i := range keys.Keys {
		switch keys.Keys[i].Key.(type) {
		case ed25519.PrivateKey:
			pk = keys.Keys[i]
		case ed25519.PublicKey:
			kid = keys.Keys[i].KeyID

		case *ecdsa.PrivateKey:
			pk = keys.Keys[i]
		case *ecdsa.PublicKey:
			kid = keys.Keys[i].KeyID

		case *rsa.PrivateKey:
			pk = keys.Keys[i]
		case *rsa.PublicKey:
			kid = keys.Keys[i].KeyID

		case []byte:
			pk = keys.Keys[i]
			kid = keys.Keys[i].KeyID

		default:
			return nil, "", errors.Errorf("credentials: unknown key type '%s'", reflect.TypeOf(key))
		}

		if pk.Key != nil && kid != "" {
			break
		}
	}

	if pk.KeyID == "" {
		return nil, "", errors.Errorf("credentials: no suitable key could be found")
	}

	if kid == "" {
		kid = pk.KeyID
	}

	return &pk, kid, nil
}

func Sign(store authn.KeyStore, src *url.URL, claims jwt.Claims) (string, error) {
	k, id, err := key(store, src)
	if err != nil {
		return "", err
	}

	method := jwt.GetSigningMethod(k.Algorithm)
	if method == nil {
		return "", errors.Errorf(`credentials: signing key "%s" declares unsupported algorithm "%s"`, k.KeyID, k.Algorithm)
	}

	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = id

	signed, err := token.SignedString(k.Key)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return signed, nil
}

func MustSign(store authn.KeyStore, src *url.URL, claims jwt.Claims) string {
	token, err := Sign(store, src, claims)
	if err != nil {
		panic(fmt.Sprintf("failed mustSign with: %s", err))
	}

	return token
}
