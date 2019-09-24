// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package opa

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/plugins/discovery"
	"github.com/open-policy-agent/opa/plugins/logs"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/server"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/pkg/errors"
)

// OPA represents an instance of the policy engine.
type OPA struct {
	decision    string
	configBytes []byte
	manager     *plugins.Manager
}

// ConfigOPA sets the configuration file to use on the OPA instance.
func ConfigOPA(fileName string) func(opa *OPA) error {
	return func(opa *OPA) error {
		bs, err := ioutil.ReadFile(fileName)
		if err != nil {
			return err
		}
		opa.configBytes = bs
		return nil
	}
}

// New returns a new OPA object.
func NewOPA(opts ...func(*OPA) error) (*OPA, error) {

	opa := &OPA{}

	for _, opt := range opts {
		if err := opt(opa); err != nil {
			return nil, err
		}
	}

	store := inmem.New()

	id, err := uuid4()
	if err != nil {
		return nil, err
	}

	opa.manager, err = plugins.New(opa.configBytes, id, store)
	if err != nil {
		return nil, err
	}

	disc, err := discovery.New(opa.manager)
	if err != nil {
		return nil, err
	}

	opa.manager.Register("discovery", disc)

	return opa, nil
}

// Start asynchronously starts the policy engine's plugins that download
// policies, report status, etc.
func (opa *OPA) Start(ctx context.Context) error {
	return opa.manager.Start(ctx)
}

// Bool returns a boolean policy decision.
func (opa *OPA) PartialEvaluate(ctx context.Context, input interface{}, query string, opts ...func(*rego.Rego)) (*rego.PartialQueries, error) {

	m := metrics.New()
	var decisionID string
	var partialResult *rego.PartialQueries

	err := storage.Txn(ctx, opa.manager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {

		var err error
		decisionID, err = uuid4()
		if err != nil {
			return err
		}

		r := rego.New(append(opts,
			rego.Metrics(m),
			rego.Query(query),
			rego.Input(input),
			rego.Compiler(opa.manager.GetCompiler()),
			rego.Store(opa.manager.Store),
			rego.Transaction(txn))...)

		rs, err := r.Partial(ctx)

		if err != nil {
			return err
		} else {
			partialResult = rs
		}

		return nil
	})

	if logger := logs.Lookup(opa.manager); logger != nil {
		record := &server.Info{
			DecisionID: decisionID,
			Timestamp:  time.Now(),
			Error:      err,
			Metrics:    m,
		}

		if err := logger.Log(ctx, record); err != nil {
			return partialResult, errors.Wrap(err, "failed to log decision")
		}
	}

	return partialResult, err
}

func uuid4() (string, error) {
	bs := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, bs)
	if n != len(bs) || err != nil {
		return "", err
	}
	bs[8] = bs[8]&^0xc0 | 0x80
	bs[6] = bs[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", bs[0:4], bs[4:6], bs[6:8], bs[8:10], bs[10:]), nil
}
