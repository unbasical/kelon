package integration

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/authn"
	"gopkg.in/square/go-jose.v2"
	"net/url"
	"reflect"
	"testing"
)

func key(store authn.KeyStore, src url.URL) (*jose.JSONWebKey, string, error) {
	keys, err := store.ResolveKeySet(src)
	if err != nil {
		return nil, "", err
	}

	var pk jose.JSONWebKey
	var kid string
	for _, key := range keys.Keys {
		switch key.Key.(type) {
		case ed25519.PrivateKey:
			pk = key
		case ed25519.PublicKey:
			kid = key.KeyID

		case *ecdsa.PrivateKey:
			pk = key
		case *ecdsa.PublicKey:
			kid = key.KeyID

		case *rsa.PrivateKey:
			pk = key
		case *rsa.PublicKey:
			kid = key.KeyID

		case []byte:
			pk = key
			kid = key.KeyID

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

func sign(store authn.KeyStore, src url.URL, claims jwt.Claims) (string, error) {
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

func mustSign(t *testing.T, store authn.KeyStore, src url.URL, claims jwt.Claims) string {
	token, err := sign(store, src, claims)
	if err != nil {
		t.Errorf("failed mustSign with: %s", err)
		t.FailNow()
	}

	return token
}
