package translate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/unbasical/kelon/internal/pkg/util"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"github.com/unbasical/kelon/pkg/data"
	internalErrors "github.com/unbasical/kelon/pkg/errors"
)

type astProcessor struct {
	fromEntity   *data.Entity
	link         map[string]interface{}
	conjunctions []data.Node
	entities     map[string]interface{}
	relations    []data.Node
	operands     util.Stack[[]data.Node]
	errors       []string
	skipUnknown  bool
	validateMode bool
}

func newAstProcessor(skipUnknown, validateMode bool) *astProcessor {
	return &astProcessor{skipUnknown: skipUnknown, validateMode: validateMode}
}

// See translate.AstTranslator.
func (p *astProcessor) Process(ctx context.Context, query ast.Body) (data.Node, error) {
	p.link = make(map[string]interface{})
	p.conjunctions = []data.Node{}
	p.entities = make(map[string]interface{})
	p.relations = []data.Node{}
	p.operands = util.Stack[[]data.Node]{}
	p.errors = []string{}

	// NEW ERA
	var clause data.Node
	p.translateQuery(query)
	condition := data.Condition{Clause: data.Conjunction{Clauses: append(p.conjunctions[:0:0], p.conjunctions...)}}

	// Add new Query
	delete(p.link, p.fromEntity.String())
	clause = data.Query{
		From:      *p.fromEntity,
		Link:      toDataLink(p.link),
		Condition: condition,
	}

	// Cleanup
	p.conjunctions = p.conjunctions[:0]
	p.fromEntity = nil
	p.link = make(map[string]interface{})

	// AST transformation did produce errors
	if len(p.errors) > 0 {
		return nil, internalErrors.InvalidRequestTranslation{Causes: p.errors}
	}

	return clause, nil
}

func toDataLink(linkedEntities map[string]interface{}) data.Link {
	entities := make([]data.Entity, len(linkedEntities))
	for i, e := range keys(linkedEntities) {
		entities[i] = data.Entity{Value: e}
	}
	return data.Link{Entities: entities}
}

// Implementation of the visitor pattern to crawl the AST.
func (p *astProcessor) Visit(v interface{}) ast.Visitor {
	switch node := v.(type) {
	case *ast.Body:
		logging.LogForComponent("astProcessor").Debugf("Body: -> %+v", v)
		return p.translateQuery(*node)
	case *ast.Expr:
		logging.LogForComponent("astProcessor").Debugf("Expr: -> %+v", v)
		return p.translateExpr(*node)
	case *ast.Term:
		logging.LogForComponent("astProcessor").Debugf("Term: -> %+v", v)
		return p.translateTerm(*node)
	default:
		if p.skipUnknown || p.validateMode {
			logging.LogForComponent("astProcessor").Warnf("Unexpectedly visiting children of: %T -> %+v", v, v)
		}
		// If not skipping unknown ast nodes -> error
		if !p.skipUnknown {
			p.errors = append(p.errors, fmt.Sprintf("Unexpectedly visiting children of: %T -> %+v", v, v))
		}
	}
	return p
}

func (p *astProcessor) translateQuery(q ast.Body) ast.Visitor {
	logging.LogForComponent("astProcessor").Debugf("================= PROCESS QUERY: %+v", q)
	for _, exp := range q {
		ast.Walk(p, exp)
	}

	logging.LogForComponent("astProcessor").Debugf("%30sAppend to Conjunctions -> %+v", "", p.relations)
	p.conjunctions = append(p.conjunctions, p.relations...)

	logging.LogForComponent("astProcessor").Debugf("%30sClean entities and relations", "")
	p.entities = make(map[string]interface{})
	p.relations = p.relations[:0]

	return p
}

func (p *astProcessor) translateExpr(node ast.Expr) ast.Visitor {
	if !node.IsCall() {
		return p
	}

	op := data.Operator{Value: node.Operator().String()}
	p.operands.Push([]data.Node{})
	for _, term := range node.Operands() {
		ast.Walk(p, term)
	}

	functionOperands, err := p.operands.Pop()
	if err != nil {
		logging.LogForComponent("astProcessor").Panicf("Error popping operands: %s", err)
	}
	if len(p.entities) > 1 {
		for _, entity := range keys(p.entities) {
			p.link[entity] = true
		}
		logging.LogForComponent("astProcessor").Debugf("%30sLink: %+v", "", p.link)
	}
	// Append new relation for conjunction
	p.relations = append(p.relations, data.Call{
		Operator: op,
		Operands: functionOperands,
	})
	logging.LogForComponent("astProcessor").Debugf("%30sRelations: %+v", "", p.relations)

	// Cleanup
	p.entities = make(map[string]interface{})
	return nil
}

func (p *astProcessor) translateTerm(node ast.Term) ast.Visitor {
	switch v := node.Value.(type) {
	case ast.Boolean:
		err := util.AppendToTop(&p.operands, makeConstant(v.String()))
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error appending to top: %s", err)
		}
		return nil
	case ast.String:
		err := util.AppendToTop(&p.operands, makeConstant(v.String()))
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error appending to top: %s", err)
		}
		return nil
	case ast.Number:
		err := util.AppendToTop(&p.operands, makeConstant(v.String()))
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error appending to top: %s", err)
		}
		return nil
	case ast.Ref:
		if len(v) == 3 {
			entity := data.Entity{Value: normalizeString(v[1].Value.String())}
			p.entities[entity.Value] = nil
			if p.fromEntity == nil {
				p.fromEntity = &entity
			}
			attribute := data.Attribute{Entity: entity, Name: normalizeString(v[2].Value.String())}
			err := util.AppendToTop(&p.operands, data.Node(attribute))
			if err != nil {
				logging.LogForComponent("astProcessor").Panicf("Error appending to top: %s", err)
			}
		}
		return nil
	case ast.Call:
		op := data.Operator{Value: v[0].String()}
		p.operands.Push([]data.Node{})
		for _, term := range v[1:] {
			ast.Walk(p, term)
		}

		functionOperands, err := p.operands.Pop()
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error popping operands: %s", err)
		}
		err = util.AppendToTop(&p.operands, data.Node(data.Call{
			Operator: op,
			Operands: functionOperands,
		}))
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error appending to top: %s", err)
		}
		return nil
	default:
		if p.skipUnknown || p.validateMode {
			logging.LogForComponent("astProcessor").Warnf("Unexpected term Node: %T -> %+v", v, v)
		}
		// If not skipping unknown ast nodes -> error
		if !p.skipUnknown {
			p.errors = append(p.errors, fmt.Sprintf("Unexpected term Node: %T -> %+v", v, v))
		}
	}
	return p
}

func makeConstant(value string) data.Node {
	// Convert "<Const>" to <Const>
	value = trimLeadingAndTrailing(value, '"')

	// Const is int
	if num, err := strconv.Atoi(value); err == nil {
		return &data.Constant{
			Value:     fmt.Sprintf("%d", num),
			IsNumeric: true,
			IsInt:     true,
			IsFloat32: false,
		}
	}

	// Const is float
	if num, err := strconv.ParseFloat(value, 32); err == nil {
		return &data.Constant{
			Value:     fmt.Sprintf("%f", num),
			IsNumeric: true,
			IsInt:     false,
			IsFloat32: true,
		}
	}

	// Const is string
	return &data.Constant{
		Value:     value,
		IsNumeric: false,
		IsInt:     false,
		IsFloat32: false,
	}
}

func normalizeString(value string) string {
	return strings.ReplaceAll(value, "\"", "")
}

func trimLeadingAndTrailing(s string, r rune) string {
	if len(s) < 2 {
		return s
	}

	if rune(s[0]) == r && rune(s[len(s)-1]) == r {
		return s[1 : len(s)-1]
	}

	return s
}

func keys(input map[string]interface{}) []string {
	i := 0
	result := make([]string, len(input))
	for k := range input {
		result[i] = k
		i++
	}
	return result
}
