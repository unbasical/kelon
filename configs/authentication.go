package configs

import (
	"github.com/pkg/errors"
	"net/url"
	"time"
)

type JwtAuthentication struct {
	JwksStringURLs    []string      `yaml:"jwks_urls"`
	JwksMaxWait       time.Duration `yaml:"jwks_max_wait"`
	JwksTTL           time.Duration `yaml:"jwks_ttl"`
	TargetAudience    []string      `yaml:"target_audience"`
	TrustedIssuers    []string      `yaml:"trusted_issuers"`
	AllowedAlgorithms []string      `yaml:"allowed_algorithms"`
	RequiredScopes    []string      `yaml:"required_scopes"`
	ScopeStrategy     string        `yaml:"scope_strategy"`
	TokenFrom         string        `yaml:"token_from"`
	JwksUrls          []url.URL     `yaml:"-"`
}

func (c *JwtAuthentication) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var partial struct {
		JwksStringURLs    []string      `yaml:"jwks_urls"`
		JwksMaxWait       time.Duration `yaml:"jwks_max_wait"`
		JwksTTL           time.Duration `yaml:"jwks_ttl"`
		TargetAudience    []string      `yaml:"target_audience"`
		TrustedIssuers    []string      `yaml:"trusted_issuers"`
		AllowedAlgorithms []string      `yaml:"allowed_algorithms"`
		RequiredScopes    []string      `yaml:"required_scopes"`
		ScopeStrategy     string        `yaml:"scope_strategy"`
	}

	err := unmarshal(&partial)
	if err != nil {
		return err
	}

	c.JwksStringURLs = partial.JwksStringURLs
	c.JwksMaxWait = partial.JwksMaxWait
	c.JwksTTL = partial.JwksTTL
	c.TargetAudience = partial.TargetAudience
	c.TrustedIssuers = partial.TrustedIssuers
	c.AllowedAlgorithms = partial.AllowedAlgorithms
	c.RequiredScopes = partial.RequiredScopes
	c.ScopeStrategy = partial.ScopeStrategy
	c.JwksUrls = make([]url.URL, 0, len(c.JwksStringURLs))

	for _, strUrl := range c.JwksStringURLs {
		u, urlErr := url.Parse(strUrl)
		if urlErr != nil {
			return errors.Wrapf(err, "unable to parse [%s] to url", strUrl)
		}

		c.JwksUrls = append(c.JwksUrls, *u)
	}

	return nil
}
