package integration

import (
	"context"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/unbasical/kelon/configs"
	authnint "github.com/unbasical/kelon/internal/pkg/authn"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/authn"
	"net/url"
	"testing"
	"time"
)

const (
	Empty            = "empty"
	Hs               = "hs"
	HsNoStrategy     = "hs-no-strategy"
	HsRequiredScopes = "hs-required-scopes"
	RSASingle        = "rsa-single"
)

var (
	UrlHs        = util.RelativeFileURLToAbsolute(*util.MustParseURL("file://config/jwks/jwks-hs.json"))
	UrlRSASingle = util.RelativeFileURLToAbsolute(*util.MustParseURL("file://config/jwks/jwks-rsa-single.json"))
)

func Test_integration_jwt(t *testing.T) {

	authConfigs := map[string]configs.JwtAuthentication{
		Empty: {
			JwksMaxWait:   time.Millisecond * 100,
			JwksTTL:       time.Minute * 30,
			ScopeStrategy: "exact",
			JwksUrls:      []url.URL{},
		},
		Hs: {
			JwksMaxWait:       time.Millisecond * 100,
			JwksTTL:           time.Minute * 30,
			AllowedAlgorithms: []string{"HS256"},
			TrustedIssuers:    []string{"iss-1", "iss-2"},
			TargetAudience:    []string{"aud-1", "aud-2"},
			ScopeStrategy:     "exact",
			JwksUrls:          []url.URL{UrlHs},
		},
		HsRequiredScopes: {
			JwksMaxWait:       time.Millisecond * 100,
			JwksTTL:           time.Minute * 30,
			AllowedAlgorithms: []string{"HS256"},
			RequiredScopes:    []string{"required"},
			ScopeStrategy:     "exact",
			JwksUrls:          []url.URL{UrlHs},
		},
		HsNoStrategy: {
			JwksMaxWait:       time.Millisecond * 100,
			JwksTTL:           time.Minute * 30,
			AllowedAlgorithms: []string{"HS256"},
			JwksUrls:          []url.URL{UrlHs},
		},
		RSASingle: {
			JwksMaxWait:   time.Millisecond * 100,
			JwksTTL:       time.Minute * 30,
			ScopeStrategy: "exact",
			JwksUrls:      []url.URL{UrlRSASingle},
		},
	}
	auths := initAuths(t, authConfigs)

	now := time.Now()

	runs := []struct {
		Name        string
		Config      string
		Token       string
		Scopes      []string
		ExpectValid bool
	}{
		{
			Name:        "should fail because JWT is invalid",
			Token:       "invalid",
			Config:      Empty,
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should pass because JWT is valid",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":   "sub",
					"exp":   now.Add(time.Hour).Unix(),
					"aud":   []string{"aud-1", "aud-2"},
					"iss":   "iss-2",
					"scope": []string{"scope-3", "scope-2", "scope-1"},
				}),
			ExpectValid: true,
		},
		{
			Config: Hs,
			Name:   "should pass even when scope is a string",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":   "sub",
					"exp":   now.Add(time.Hour).Unix(),
					"aud":   []string{"aud-1", "aud-2"},
					"iss":   "iss-2",
					"scope": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: true,
		},
		{
			Config: Hs,
			Name:   "should pass when scope is keyed as scp",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub": "sub",
					"exp": now.Add(time.Hour).Unix(),
					"aud": []string{"aud-1", "aud-2"},
					"iss": "iss-2",
					"scp": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: true,
		},
		{
			Config: Hs,
			Name:   "should pass when scope is keyed as scopes",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-1", "aud-2"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: true,
		},
		{
			Config: HsRequiredScopes,
			Name:   "should pass with required scope",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-1", "aud-2"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1 required",
				}),
			ExpectValid: true,
		},
		{
			Config: HsRequiredScopes,
			Name:   "should fail with required scope missing",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-1", "aud-2"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
		{
			Config: HsNoStrategy,
			Name:   "should fail when scope validation was requested but no scope strategy is set",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, HsNoStrategy),
				UrlHs,
				jwt.MapClaims{
					"sub":   "sub",
					"exp":   now.Add(time.Hour).Unix(),
					"aud":   []string{"aud-1", "aud-2"},
					"iss":   "iss-2",
					"scope": []string{"scope-3", "scope-2", "scope-1"},
				}),
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should fail when audience mismatches",
			Scopes: []string{"scope-1", "scope-2"},
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-3", "aud-4"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should fail when iat in future",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"iat":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-3", "aud-4"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should fail when nbf in future",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(time.Hour).Unix(),
					"nbf":    now.Add(time.Hour).Unix(),
					"aud":    []string{"aud-3", "aud-4"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should fail when expired",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(-time.Hour).Unix(),
					"aud":    []string{"aud-3", "aud-4"},
					"iss":    "iss-2",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
		{
			Config: Hs,
			Name:   "should fail when issuer mismatches",
			Token: mustSign(t, retrieveKeyStore(t, auths, Hs),
				UrlHs,
				jwt.MapClaims{
					"sub":    "sub",
					"exp":    now.Add(-time.Hour).Unix(),
					"aud":    []string{"aud-3", "aud-4"},
					"iss":    "not-iss-1",
					"scopes": "scope-3 scope-2 scope-1",
				}),
			ExpectValid: false,
		},
	}

	for _, r := range runs {
		t.Run(r.Name, func(t *testing.T) {
			fmt.Printf("TEST: %s - Token: %s\n", r.Name, r.Token)

			valid, err := auths[r.Config].Authenticate(context.Background(), r.Token, r.Scopes...)
			if err != nil && r.ExpectValid {
				t.Error(err)
				t.FailNow()
			}

			assert.Equal(t, r.ExpectValid, valid)
		})
	}
}

func initAuths(t *testing.T, authConfigs map[string]configs.JwtAuthentication) map[string]authn.Authenticator {
	auths := make(map[string]authn.Authenticator)

	for alias, config := range authConfigs {
		for i, u := range config.JwksUrls {
			uu := util.RelativeFileURLToAbsolute(u)

			config.JwksUrls[i] = uu
		}

		auths[alias] = authnint.NewJwtAuthenticator()
		err := auths[alias].Configure(context.Background(), config, "testing")
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}

	return auths
}

func retrieveKeyStore(t *testing.T, auths map[string]authn.Authenticator, alias string) authn.KeyStore {
	a, ok := auths[alias]
	if !ok {
		t.Errorf("unknown authenticator for alias [%s]", alias)
		t.FailNow()
	}

	return a.KeyStore()
}
