package authn

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/ory/x/jwtx"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/authn"
	"strings"
)

type JwtAuthenticator struct {
	configured    bool
	alias         string
	keystore      authn.KeyStore
	scopeStrategy authn.ScopeStrategy
	config        *configs.JwtAuthentication
}

func NewJwtAuthenticator() authn.Authenticator {
	return &JwtAuthenticator{}
}

func (a *JwtAuthenticator) Alias() string {
	if !a.configured {
		return ""
	}
	return a.alias
}

func (a *JwtAuthenticator) KeyStore() authn.KeyStore {
	if a.configured {
		return a.keystore
	}

	return nil
}

func (a *JwtAuthenticator) Configure(ctx context.Context, config interface{}, alias string) error {
	if a.configured {
		return nil
	}

	jwtConfig, ok := config.(configs.JwtAuthentication)
	if !ok {
		return errors.Errorf("expected config of type %T but got %T", configs.JwtAuthentication{}, config)
	}

	a.alias = alias
	a.config = &jwtConfig
	a.keystore = NewDefaultKeyStore(ctx, a.config.JwksTTL, a.config.JwksMaxWait)

	for _, jwksUrl := range a.config.JwksUrls {
		err := a.keystore.Register(jwksUrl)
		if err != nil {
			return errors.Wrapf(err, "failed to register url [%s]", jwksUrl.String())
		}
	}

	switch strings.ToLower(a.config.ScopeStrategy) {
	case "hierarchic":
		a.scopeStrategy = authn.HierarchicScopeStrategy
	case "exact":
		a.scopeStrategy = authn.ExactScopeStrategy
	case "wildcard":
		a.scopeStrategy = authn.WildcardScopeStrategy
	default:
		a.scopeStrategy = nil
	}

	a.configured = true

	return nil
}

func (a *JwtAuthenticator) Authenticate(ctx context.Context, token string, scopes ...string) (bool, error) {
	t, err := jwt.ParseWithClaims(token, jwt.MapClaims{}, a.keyFunc)

	if err != nil {
		return false, err
	} else if !t.Valid {
		return false, nil
	}

	mapClaims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return false, errors.New("Unable to type assert jwt claims to jwt.MapClaims.")
	}

	parsedClaims := jwtx.ParseMapStringInterfaceClaims(mapClaims)
	audienceValid, notContained := util.SliceContainsSlice(parsedClaims.Audience, a.config.TargetAudience)
	if !audienceValid {
		return false, errors.Errorf("token does not have all audiences, missing audiences: %v", notContained)
	}

	if len(a.config.TrustedIssuers) > 0 {
		if !util.SliceContains(a.config.TrustedIssuers, parsedClaims.Issuer) {
			return false, errors.Errorf("Token issuer does not match any trusted issuer [%s]. received issuers: [%s]", parsedClaims.Issuer, strings.Join(a.config.TrustedIssuers, ", "))
		}
	}

	s, k := scope(mapClaims)
	delete(mapClaims, k)
	mapClaims["scp"] = s

	requiredScopes := util.SliceMerge(scopes, a.config.RequiredScopes)

	if a.scopeStrategy != nil {
		for _, sc := range requiredScopes {
			if !a.scopeStrategy(s, sc) {
				return false, errors.Errorf("JSON Web Token is missing required scope [%s]", sc)
			}
		}
	} else {
		if len(scopes) > 0 {
			return false, errors.Errorf("Scope validation was requested but scope strategy is set to [none]")
		}
	}

	return true, nil
}

func (a *JwtAuthenticator) keyFunc(token *jwt.Token) (interface{}, error) {
	if !util.SliceContains(a.config.AllowedAlgorithms, fmt.Sprintf("%s", token.Header["alg"])) {
		return nil, errors.Errorf("JSON Web Token used signing method [%s] which is not allowed", token.Header["alg"])
	}

	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errors.New("The JSON Web Token must contain a kid header value but did not.")
	}

	key, err := a.keystore.ResolveKey(a.config.JwksUrls, kid, "sig")
	if err != nil {
		return nil, err
	}

	// Mutate to public key
	if _, ok := key.Key.([]byte); !ok && !key.IsPublic() {
		k := key.Public()
		key = &k
	}

	switch token.Method.(type) {
	case *jwt.SigningMethodRSA:
		if k, ok := key.Key.(*rsa.PublicKey); ok {
			return k, nil
		}
	case *jwt.SigningMethodECDSA:
		if k, ok := key.Key.(*ecdsa.PublicKey); ok {
			return k, nil
		}
	case *jwt.SigningMethodRSAPSS:
		if k, ok := key.Key.(*rsa.PublicKey); ok {
			return k, nil
		}
	case *jwt.SigningMethodHMAC:
		if k, ok := key.Key.([]byte); ok {
			return k, nil
		}
	default:
		return nil, errors.Errorf("This request object uses unsupported signing algorithm [%s]", token.Header["alg"])
	}

	return nil, errors.Errorf("The signing key algorithm does not match the algorithm from the token header")
}

func scope(claims map[string]interface{}) ([]string, string) {
	var ok bool
	var interim interface{}
	var key string

	for _, k := range []string{"scp", "scope", "scopes"} {
		if interim, ok = claims[k]; ok {
			key = k
			break
		}
	}

	if !ok {
		return []string{}, key
	}

	switch i := interim.(type) {
	case []string:
		return i, key
	case []interface{}:
		vs := make([]string, len(i))
		for k, v := range i {
			if vv, ok := v.(string); ok {
				vs[k] = vv
			}
		}
		return vs, key
	case string:
		return strings.Split(i, " "), key
	default:
		return []string{}, key
	}
}
