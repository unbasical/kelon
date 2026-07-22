package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/data"
)

func newTestSpiceDBTranslator(t *testing.T) data.DatastoreTranslator {
	t.Helper()
	appConf := &configs.AppConfig{
		ExternalConfig: configs.ExternalConfig{
			Datastores: map[string]*configs.Datastore{
				"spicedb": {
					Type:       data.TypeSpicedb,
					Connection: map[string]string{"endpoint": "http://localhost:8443", "token": "test-token"},
					Metadata:   map[string]string{},
				},
			},
			DatastoreSchemas: map[string]map[string]*configs.EntitySchema{
				"spicedb": {
					"authz": {
						Entities: []*configs.Entity{
							{Name: "permissions"},
						},
					},
				},
			},
		},
	}

	translator := NewSpiceDBDatastoreTranslator()
	assert.NoError(t, translator.Configure(appConf, "spicedb"))
	return translator
}

func spiceDBEqCall(attribute string, value data.Node) data.Call {
	return data.Call{
		Operator: data.Operator{Value: "eq"},
		Operands: []data.Node{
			data.Attribute{Entity: data.Entity{Value: "permissions"}, Name: attribute},
			value,
		},
	}
}

func spiceDBQueryNode(calls ...data.Node) data.Query {
	return data.Query{
		From:      data.Entity{Value: "permissions"},
		Condition: data.Condition{Clause: data.Conjunction{Clauses: calls}},
	}
}

func Test_SpiceDBTranslator_SingleQuery(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	// Pointer constants, attribute-first operand order
	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			spiceDBEqCall("object", &data.Constant{Value: "asset:a1"}),
			spiceDBEqCall("permission", &data.Constant{Value: "view"}),
			spiceDBEqCall("subject", &data.Constant{Value: "user:alice"}),
		),
	}}

	result, err := translator.Execute(t.Context(), query)
	assert.NoError(t, err)
	assert.Equal(t, []SpiceDBCheck{
		{Object: "asset:a1", Permission: "view", Subject: "user:alice"},
	}, result.Statement)
	assert.Nil(t, result.Parameters)
}

func Test_SpiceDBTranslator_MultipleQueries(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	// Second clause uses value constants and constant-first operand order
	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			spiceDBEqCall("object", &data.Constant{Value: "asset:a1"}),
			spiceDBEqCall("permission", &data.Constant{Value: "view"}),
			spiceDBEqCall("subject", &data.Constant{Value: "user:alice"}),
		),
		spiceDBQueryNode(
			data.Call{
				Operator: data.Operator{Value: "eq"},
				Operands: []data.Node{
					data.Constant{Value: "group:admins"},
					data.Attribute{Entity: data.Entity{Value: "permissions"}, Name: "object"},
				},
			},
			data.Call{
				Operator: data.Operator{Value: "eq"},
				Operands: []data.Node{
					data.Constant{Value: "member"},
					data.Attribute{Entity: data.Entity{Value: "permissions"}, Name: "permission"},
				},
			},
			spiceDBEqCall("subject", data.Constant{Value: "user:bob"}),
		),
	}}

	result, err := translator.Execute(t.Context(), query)
	assert.NoError(t, err)
	assert.Equal(t, []SpiceDBCheck{
		{Object: "asset:a1", Permission: "view", Subject: "user:alice"},
		{Object: "group:admins", Permission: "member", Subject: "user:bob"},
	}, result.Statement)
}

func Test_SpiceDBTranslator_SingleCallCondition(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	// A condition consisting of a single call (no conjunction) is missing two attributes
	query := data.Union{Clauses: []data.Node{
		data.Query{
			From:      data.Entity{Value: "permissions"},
			Condition: data.Condition{Clause: spiceDBEqCall("object", &data.Constant{Value: "asset:a1"})},
		},
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "Missing attribute [permission]")
}

func Test_SpiceDBTranslator_EmptyUnion(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	result, err := translator.Execute(t.Context(), data.Union{Clauses: []data.Node{}})
	assert.NoError(t, err)
	assert.Equal(t, []SpiceDBCheck{}, result.Statement)
}

func Test_SpiceDBTranslator_UnsupportedOperator(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			data.Call{
				Operator: data.Operator{Value: "gt"},
				Operands: []data.Node{
					data.Attribute{Entity: data.Entity{Value: "permissions"}, Name: "object"},
					&data.Constant{Value: "asset:a1"},
				},
			},
		),
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "Operator [gt] is not supported")
}

func Test_SpiceDBTranslator_UnknownAttribute(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			spiceDBEqCall("object", &data.Constant{Value: "asset:a1"}),
			spiceDBEqCall("permission", &data.Constant{Value: "view"}),
			spiceDBEqCall("owner", &data.Constant{Value: "user:alice"}),
		),
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "Unknown attribute [owner]")
}

func Test_SpiceDBTranslator_MissingAttribute(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			spiceDBEqCall("object", &data.Constant{Value: "asset:a1"}),
			spiceDBEqCall("permission", &data.Constant{Value: "view"}),
		),
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "Missing attribute [subject]")
}

func Test_SpiceDBTranslator_DuplicateAttributeWithDifferentValue(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			spiceDBEqCall("object", &data.Constant{Value: "asset:a1"}),
			spiceDBEqCall("object", &data.Constant{Value: "asset:a2"}),
			spiceDBEqCall("permission", &data.Constant{Value: "view"}),
			spiceDBEqCall("subject", &data.Constant{Value: "user:alice"}),
		),
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "constrained twice")
}

func Test_SpiceDBTranslator_JoinsUnsupported(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		data.Query{
			From:      data.Entity{Value: "permissions"},
			Link:      data.Link{Entities: []data.Entity{{Value: "other"}}},
			Condition: data.Condition{Clause: data.Conjunction{Clauses: []data.Node{}}},
		},
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "spicedb supports no joins")
}

func Test_SpiceDBTranslator_UnknownEntity(t *testing.T) {
	translator := newTestSpiceDBTranslator(t)

	query := data.Union{Clauses: []data.Node{
		spiceDBQueryNode(
			data.Call{
				Operator: data.Operator{Value: "eq"},
				Operands: []data.Node{
					data.Attribute{Entity: data.Entity{Value: "unknown"}, Name: "object"},
					&data.Constant{Value: "asset:a1"},
				},
			},
		),
	}}

	_, err := translator.Execute(t.Context(), query)
	assert.ErrorContains(t, err, "Entity [unknown] is not configured")
}

func Test_SpiceDBTranslator_NotConfigured(t *testing.T) {
	translator := NewSpiceDBDatastoreTranslator()

	_, err := translator.Execute(context.Background(), data.Union{})
	assert.ErrorContains(t, err, "was not configured")
}
