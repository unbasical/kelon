package data

import (
	"context"

	"github.com/pkg/errors"
	"github.com/unbasical/kelon/configs"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
)

// Attribute names every SpiceDB check clause has to provide.
const (
	spiceDBAttrObject     = "object"
	spiceDBAttrPermission = "permission"
	spiceDBAttrSubject    = "subject"
)

// SpiceDBCheck is a single SpiceDB CheckPermission request descriptor.
// Object and Subject are full SpiceDB object references of the form "<type>:<id>".
type SpiceDBCheck struct {
	Object     string `json:"object"`
	Permission string `json:"permission"`
	Subject    string `json:"subject"`
}

type spiceDBDatastoreTranslator struct {
	appConf    *configs.AppConfig
	alias      string
	entities   map[string]bool
	configured bool
}

// NewSpiceDBDatastoreTranslator returns a new data.DatastoreTranslator which translates the Query-AST
// into a list of SpiceDB CheckPermission request descriptors ([]SpiceDBCheck).
func NewSpiceDBDatastoreTranslator() data.DatastoreTranslator {
	return &spiceDBDatastoreTranslator{
		appConf:    nil,
		alias:      "",
		entities:   nil,
		configured: false,
	}
}

// Configure -- see data.DatastoreTranslator
func (ds *spiceDBDatastoreTranslator) Configure(appConf *configs.AppConfig, alias string) error {
	// Exit if already configured
	if ds.configured {
		return nil
	}

	if appConf == nil {
		return errors.Errorf("SpiceDBDatastoreTranslator: AppConfig not configured!")
	}
	if alias == "" {
		return errors.Errorf("SpiceDBDatastoreTranslator: Empty alias provided!")
	}
	if _, ok := appConf.Datastores[alias]; !ok {
		return errors.Errorf("SpiceDBDatastoreTranslator: No datastore with alias [%s] configured!", alias)
	}

	// Load entity vocabulary from the configured schemas
	schemas, ok := appConf.DatastoreSchemas[alias]
	if !ok || len(schemas) == 0 {
		return errors.Errorf("SpiceDBDatastoreTranslator: DatastoreTranslator with alias [%s] has no entity-schema-mapping configured!", alias)
	}
	ds.entities = make(map[string]bool)
	for _, schema := range schemas {
		for _, entity := range schema.Entities {
			ds.entities[entity.Name] = true
			if entity.Alias != "" {
				ds.entities[entity.Alias] = true
			}
		}
	}

	// Assign values
	ds.appConf = appConf
	ds.alias = alias
	ds.configured = true
	logging.LogForComponent("spiceDBDatastoreTranslator").Infof("Configured [%s]", alias)
	return nil
}

// Execute -- see data.DatastoreTranslator
func (ds *spiceDBDatastoreTranslator) Execute(_ context.Context, query data.Node) (data.DatastoreQuery, error) {
	if !ds.configured {
		return data.DatastoreQuery{}, errors.Errorf("SpiceDBDatastoreTranslator: Datastore was not configured! Please call Configure().")
	}
	logging.LogForComponent("spiceDBDatastoreTranslator").Debugf("TRANSLATING QUERY: ==================%+v==================", query.String())

	union, ok := query.(data.Union)
	if !ok {
		return data.DatastoreQuery{}, errors.Errorf("SpiceDBDatastoreTranslator: Expected root node of type data.Union but got: %T -> %+v", query, query)
	}

	checks := make([]SpiceDBCheck, 0, len(union.Clauses))
	for _, clause := range union.Clauses {
		q, ok := clause.(data.Query)
		if !ok {
			return data.DatastoreQuery{}, errors.Errorf("SpiceDBDatastoreTranslator: Expected union clause of type data.Query but got: %T -> %+v", clause, clause)
		}
		check, err := ds.translateQuery(q)
		if err != nil {
			return data.DatastoreQuery{}, err
		}
		checks = append(checks, check)
	}

	logging.LogForComponent("spiceDBDatastoreTranslator").Debugf("TRANSLATED CHECKS: ==================%+v==================", checks)
	return data.DatastoreQuery{Statement: checks}, nil
}

// translateQuery extracts the (object, permission, subject) triple from a single data.Query clause.
func (ds *spiceDBDatastoreTranslator) translateQuery(query data.Query) (SpiceDBCheck, error) {
	// SpiceDB checks are single-object questions -> linked entities (joins) cannot be translated
	if len(query.Link.Entities) > 0 {
		return SpiceDBCheck{}, errors.Errorf("SpiceDBDatastoreTranslator: spicedb supports no joins, but query links entities %+v", query.Link.Entities)
	}

	attributes := make(map[string]string)
	if err := ds.collectAttributes(query.Condition.Clause, attributes); err != nil {
		return SpiceDBCheck{}, err
	}

	for _, required := range []string{spiceDBAttrObject, spiceDBAttrPermission, spiceDBAttrSubject} {
		if _, ok := attributes[required]; !ok {
			return SpiceDBCheck{}, errors.Errorf("SpiceDBDatastoreTranslator: Missing attribute [%s] in query condition! Each clause must constrain object, permission and subject.", required)
		}
	}

	return SpiceDBCheck{
		Object:     attributes[spiceDBAttrObject],
		Permission: attributes[spiceDBAttrPermission],
		Subject:    attributes[spiceDBAttrSubject],
	}, nil
}

// collectAttributes walks a condition clause (a data.Conjunction of data.Call nodes or a single data.Call)
// and collects the values of all eq-constraints into the attributes map.
func (ds *spiceDBDatastoreTranslator) collectAttributes(node data.Node, attributes map[string]string) error {
	switch n := node.(type) {
	case nil:
		return errors.Errorf("SpiceDBDatastoreTranslator: Query has no condition! Each clause must constrain object, permission and subject.")
	case data.Conjunction:
		for _, clause := range n.Clauses {
			if err := ds.collectAttributes(clause, attributes); err != nil {
				return err
			}
		}
		return nil
	case data.Call:
		return ds.collectCall(n, attributes)
	default:
		return errors.Errorf("SpiceDBDatastoreTranslator: Unexpected condition node: %T -> %+v", n, n)
	}
}

// collectCall extracts a single (attribute, constant) eq-pair from a data.Call.
func (ds *spiceDBDatastoreTranslator) collectCall(call data.Call, attributes map[string]string) error {
	if call.Operator.String() != "eq" {
		return errors.Errorf("SpiceDBDatastoreTranslator: Operator [%s] is not supported! SpiceDB checks only support [eq].", call.Operator.String())
	}
	if len(call.Operands) != 2 {
		return errors.Errorf("SpiceDBDatastoreTranslator: Operator [eq] expects exactly 2 operands but got %d!", len(call.Operands))
	}

	var (
		attribute *data.Attribute
		constant  *data.Constant
	)
	for _, operand := range call.Operands {
		switch op := operand.(type) {
		case data.Attribute:
			attribute = &op
		case data.Constant:
			constant = &op
		case *data.Constant:
			constant = op
		default:
			return errors.Errorf("SpiceDBDatastoreTranslator: Unexpected operand: %T -> %+v", op, op)
		}
	}
	if attribute == nil || constant == nil {
		return errors.Errorf("SpiceDBDatastoreTranslator: Operator [eq] expects one attribute and one constant operand, but got: %+v", call.Operands)
	}
	if !ds.entities[attribute.Entity.String()] {
		return errors.Errorf("SpiceDBDatastoreTranslator: Entity [%s] is not configured in the entity-schemas of datastore [%s]!", attribute.Entity.String(), ds.alias)
	}

	switch attribute.Name {
	case spiceDBAttrObject, spiceDBAttrPermission, spiceDBAttrSubject:
		if existing, ok := attributes[attribute.Name]; ok && existing != constant.String() {
			return errors.Errorf("SpiceDBDatastoreTranslator: Attribute [%s] is constrained twice with different values [%s] and [%s]!", attribute.Name, existing, constant.String())
		}
		attributes[attribute.Name] = constant.String()
		return nil
	default:
		return errors.Errorf("SpiceDBDatastoreTranslator: Unknown attribute [%s]! Supported attributes are [%s, %s, %s].", attribute.Name, spiceDBAttrObject, spiceDBAttrPermission, spiceDBAttrSubject)
	}
}
