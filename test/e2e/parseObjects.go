package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v4"
	authnint "github.com/unbasical/kelon/internal/pkg/authn"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/authn"
	"github.com/unbasical/kelon/test"
)

type Token struct {
	claims   jwt.MapClaims
	SignKey  *url.URL
	keyStore authn.KeyStore
}

func (t *Token) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var partial struct {
		SignKey string                 `yaml:"signKeyURL"`
		Claims  map[string]interface{} `yaml:"claims"`
	}

	err := unmarshal(&partial)
	if err != nil {
		return err
	}
	partial.Claims["exp"] = time.Now().Add(time.Hour).Unix()

	t.claims = partial.Claims
	t.SignKey = util.RelativeFileURLToAbsolute(util.MustParseURL(partial.SignKey))
	t.keyStore = authnint.NewDefaultKeyStore(context.Background(), time.Hour, time.Second)
	if err := t.keyStore.Register(t.SignKey); err != nil {
		panic(fmt.Sprintf("unable to register jwks url in keystore: %s", err))
	}

	return nil
}

func (t *Token) Sign() string {
	return test.MustSign(t.keyStore, t.SignKey, t.claims)
}

type Request struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	Body       string `yaml:"body"`
	Token      *Token `yaml:"token,omitempty"`
	StatusCode int    `yaml:"statusCode"`
}

func (r *Request) BodyWithToken() string {
	if r.Token == nil {
		return r.Body
	}

	signed := r.Token.Sign()

	var body map[string]map[string]interface{}
	err := json.Unmarshal([]byte(r.Body), &body)
	if err != nil {
		panic(fmt.Sprintf("failed parsing request body: %s", err))
	}

	body["input"]["token"] = signed
	b, err := json.Marshal(body)
	if err != nil {
		panic(fmt.Sprintf("failed marshal request body: %s", err))
	}

	return string(b)
}
