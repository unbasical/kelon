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
	"os"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/loader"
	log "github.com/sirupsen/logrus"

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
	configBytes []byte
	manager     *plugins.Manager
}

type loadResult struct {
	loader.Result
	Bundles map[string]*bundle.Bundle
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

// Returns a new OPA instance.
func NewOPA(ctx context.Context, regosPath string, opts ...func(*OPA) error) (*OPA, error) {
	opa := &OPA{}

	// Configure OPA
	for _, opt := range opts {
		if err := opt(opa); err != nil {
			return nil, err
		}
	}

	// Init store
	store := inmem.New()

	id, err := uuid4()
	if err != nil {
		return nil, errors.Wrap(err, "NewOPA: Unable to create uuid4")
	}

	opa.manager, err = plugins.New(opa.configBytes, id, store)
	if err != nil {
		return nil, errors.Wrap(err, "NewOPA: Error while creating manager plugin")
	}

	disc, err := discovery.New(opa.manager)
	if err != nil {
		return nil, errors.Wrap(err, "NewOPA: Error while creating discovery plugin")
	}
	opa.manager.Register("discovery", disc)

	// Load regos
	if err := opa.LoadRegosFromPath(ctx, regosPath); err != nil {
		return nil, errors.Wrap(err, "NewOPA: Unable to load regos")
	}

	return opa, nil
}

func (opa *OPA) LoadRegosFromPath(ctx context.Context, regosPath string) error {
	store := opa.manager.Store

	log.Debugf("Loading regos from dir: %s", regosPath)
	filter := func(abspath string, info os.FileInfo, depth int) bool {
		return !strings.HasSuffix(abspath, ".rego")
	}
	loaded, err := loadPaths([]string{regosPath}, filter, true)
	if err != nil {
		return errors.Wrap(err, "NewOPA: Error while loading rego dir")
	}
	for bundleName, loadedBundle := range loaded.Bundles {
		log.Infof("Loading Bundle: %s", bundleName)
		for _, module := range loadedBundle.Modules {
			log.Infof("Loaded Package: [%s] -> module [%s]", module.Parsed.Package.String(), module.Path)
		}
	}
	txn, err := store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		return errors.Wrap(err, "NewOPA: Error while opening transaction")
	}
	if len(loaded.Documents) > 0 {
		if err := store.Write(ctx, txn, storage.AddOp, storage.Path{}, loaded.Documents); err != nil {
			return errors.Wrap(err, "NewOPA: Error while writing document")
		}
	}
	if err := compileAndStoreInputs(ctx, store, txn, loaded, 1); err != nil {
		store.Abort(ctx, txn)
		return errors.Wrap(err, "NewOPA: Error while storing inputs")
	}
	if err := store.Commit(ctx, txn); err != nil {
		return errors.Wrap(err, "NewOPA: Error while commit")
	}

	return nil
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
		}
		partialResult = rs
		return nil
	})

	if logger := logs.Lookup(opa.manager); logger != nil {
		record := &server.Info{
			DecisionID: decisionID,
			Query:      query,
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

func compileAndStoreInputs(ctx context.Context, store storage.Store, txn storage.Transaction, loaded *loadResult, errorLimit int) error {
	policies := make(map[string]*ast.Module, len(loaded.Modules))

	for id, parsed := range loaded.Modules {
		policies[id] = parsed.Parsed
	}

	c := ast.NewCompiler().SetErrorLimit(errorLimit).WithPathConflictsCheck(storage.NonEmpty(ctx, store, txn))

	opts := &bundle.ActivateOpts{
		Ctx:          ctx,
		Store:        store,
		Txn:          txn,
		Compiler:     c,
		Metrics:      metrics.New(),
		Bundles:      loaded.Bundles,
		ExtraModules: policies,
	}

	err := bundle.Activate(opts)
	if err != nil {
		return err
	}

	// Policies in bundles will have already been added to the store, but
	// modules loaded outside of bundles will need to be added manually.
	for id, parsed := range loaded.Modules {
		if err := store.UpsertPolicy(ctx, txn, id, parsed.Raw); err != nil {
			return err
		}
	}

	return nil
}

func loadPaths(paths []string, filter loader.Filter, asBundle bool) (*loadResult, error) {
	result := &loadResult{}
	var err error

	if asBundle {
		result.Bundles = make(map[string]*bundle.Bundle, len(paths))
		for _, path := range paths {
			result.Bundles[path], err = loader.AsBundle(path)
			if err != nil {
				return nil, err
			}
		}
	} else {
		loaded, err := loader.Filtered(paths, filter)
		if err != nil {
			return nil, err
		}
		result.Modules = loaded.Modules
		result.Documents = loaded.Documents
	}

	return result, nil
}
