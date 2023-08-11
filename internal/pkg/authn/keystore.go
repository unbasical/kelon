package authn

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/authn"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"gocloud.dev/blob"
	"gopkg.in/square/go-jose.v2"
)

var wellKnownSuffix = "/.well-known/openid-configuration"

type defaultKeyStore struct {
	logger  *log.Entry
	urls    []url.URL
	keys    map[url.URL]jose.JSONWebKeySet
	bmux    *blob.URLMux
	lock    sync.RWMutex
	timeout time.Duration
	ticker  *time.Ticker
	done    chan struct{}
}

func NewDefaultKeyStore(ctx context.Context, refreshInterval, timeout time.Duration) authn.KeyStore {
	store := &defaultKeyStore{
		logger: logging.LogForComponent("remoteKeyStore"),
		urls:   []url.URL{},
		keys:   make(map[url.URL]jose.JSONWebKeySet),

		ticker:  time.NewTicker(refreshInterval),
		timeout: timeout,

		done: make(chan struct{}),
	}

	go store.start(ctx)

	return store
}

func (s *defaultKeyStore) Register(u url.URL) error {
	if isWellKnownUrl(u) {
		jwksUrl, err := extractJwksUrlsFromWellKnown(u)
		if err != nil {
			return errors.Wrapf(err, "unable to extract jwks_uri from well-known url [%s]", u.String())
		}
		u = jwksUrl
	}

	uu := util.RelativeFileURLToAbsolute(u)

	if !util.SliceContains(s.urls, uu) {
		s.urls = append(s.urls, uu)

		if err := s.fetchKeySet(uu); err != nil {
			return err
		}
	}

	return nil
}

func (s *defaultKeyStore) ResolveKeySet(u url.URL) (jose.JSONWebKeySet, error) {
	s.lock.RLock()
	set, ok := s.keys[u]
	s.lock.RUnlock()
	if !ok {
		return jose.JSONWebKeySet{}, errors.Errorf("no keyset was registered for url [%s]", u.String())
	}
	return set, nil
}

func (s *defaultKeyStore) ResolveKey(urls []url.URL, kid, use string) (*jose.JSONWebKey, error) {
	for _, u := range urls {
		s.lock.RLock()
		set := s.keys[u]
		s.lock.RUnlock()
		for _, key := range set.Key(kid) {
			if key.Use == use {
				return &key, nil
			}
		}
	}

	return nil, errors.Errorf("key with kid [%s] not found", kid)
}

func (s *defaultKeyStore) start(ctx context.Context) {
	for {
		select {
		case <-s.done:
			s.ticker.Stop()
			return
		case <-s.ticker.C:
			s.logger.Debug("Refreshing JWKS")
			go s.fetchAll(ctx)
		}
	}
}

func (s *defaultKeyStore) Stop() {
	s.done <- struct{}{}
}

func (s *defaultKeyStore) fetchAll(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	errs := make(chan error)
	done := make(chan struct{})

	go func() {
		for err := range errs {
			s.logger.WithError(err).Errorf("Unable to fetch JSON Web Key Set from remote")
		}
	}()

	go s.resolveAll(done, errs)

	select {
	case <-ctx.Done():
		s.logger.WithError(ctx.Err()).Errorf("Ignoring JSON Web Keys from at least one URI because the request timed out waiting for a response.")
	case <-done:
		// We're done!
	}
}

func (s *defaultKeyStore) resolveAll(done chan struct{}, errs chan error) {
	var wg sync.WaitGroup

	for _, u := range s.urls {
		wg.Add(1)
		go s.fetch(&wg, errs, u)
	}

	wg.Wait()
	close(done)
	close(errs)
}

func (s *defaultKeyStore) fetch(wg *sync.WaitGroup, errs chan error, u url.URL) {
	defer wg.Done()
	err := s.fetchKeySet(u)
	if err != nil {
		errs <- err
	}
}

func (s *defaultKeyStore) fetchKeySet(u url.URL) error {
	var (
		reader io.ReadCloser
		err    error
	)

	switch u.Scheme {
	case "azblob", "gs", "s3":
		ctx := context.Background()
		bucket, err := s.bmux.OpenBucket(ctx, u.Scheme+"://"+u.Host)
		if err != nil {
			return errors.Wrapf(err, "unable to fetch JSON Web Keys from location [%s]", u.String())
		}
		defer bucket.Close()

		reader, err = bucket.NewReader(ctx, u.Path[1:], nil)
		if err != nil {
			return errors.Wrapf(err, "unable to fetch JSON Web Keys from location [%s]", u.String())
		}
		defer reader.Close()

	case "http", "https":
		res, err := http.Get(u.String())
		if err != nil {
			return errors.Wrapf(err, "unable to fetch JSON Web Keys from location [%s]", u.String())
		}
		reader = res.Body
		defer reader.Close()

		if res.StatusCode < 200 || res.StatusCode >= 400 {
			return errors.Wrapf(err, "expected successful status code from location [%s], but received code %d", u.String(), res.StatusCode)
		}

	case "", "file":
		reader, err = os.Open(u.Path)
		if err != nil {
			return errors.Wrapf(err, "unable to fetch JSON Web Keys from location [%s]", u.Path)
		}
		defer reader.Close()

	default:
		return errors.Errorf("unable to fetch JSON Web Keys from location [%s] because URL scheme [%s] is not supported", u.String(), u.Scheme)
	}

	jwks, err := jwksFromReader(reader)
	if err != nil {
		return errors.Errorf("unable to parse resource to JSON Web Keys linked by [%s]", u.String())
	}

	s.lock.Lock()
	s.keys[u] = jwks
	s.lock.Unlock()

	return nil
}

func jwksFromReader(reader io.Reader) (jose.JSONWebKeySet, error) {
	var jwks jose.JSONWebKeySet
	err := json.NewDecoder(reader).Decode(&jwks)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	return jwks, nil
}

func isWellKnownUrl(u url.URL) bool {
	return strings.HasSuffix(u.String(), wellKnownSuffix)
}

func extractJwksUrlsFromWellKnown(u url.URL) (url.URL, error) {
	response, err := http.Get(u.String())
	if err != nil {
		return url.URL{}, err
	}
	defer response.Body.Close()

	var oidcResponse authn.OIDCWellKnownResponse
	err = json.NewDecoder(response.Body).Decode(&oidcResponse)
	if err != nil {
		return url.URL{}, err
	}

	return oidcResponse.JwksUri, nil
}
