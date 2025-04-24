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
	visitor      *ast.GenericVisitor
	fromEntity   *data.Entity
	link         map[string]any
	conjunctions []data.Node
	entities     map[string]any
	relations    []data.Node
	operands     util.Stack[[]data.Node]
	errors       []string
	skipUnknown  bool
	validateMode bool
}

func newAstProcessor(skipUnknown, validateMode bool) *astProcessor {
	processor := &astProcessor{skipUnknown: skipUnknown, validateMode: validateMode}
	processor.visitor = ast.NewGenericVisitor(processor.Visit)

	return processor
}

// Process --  See translate.AstTranslator.
func (p *astProcessor) Process(_ context.Context, query ast.Body) (data.Node, error) {
	p.link = make(map[string]any)
	p.conjunctions = []data.Node{}
	p.entities = make(map[string]any)
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
	p.link = make(map[string]any)

	// AST transformation did produce errors
	if len(p.errors) > 0 {
		return nil, internalErrors.InvalidRequestTranslation{Causes: p.errors}
	}

	return clause, nil
}

func toDataLink(linkedEntities map[string]any) data.Link {
	entities := make([]data.Entity, len(linkedEntities))
	for i, e := range keys(linkedEntities) {
		entities[i] = data.Entity{Value: e}
	}
	return data.Link{Entities: entities}
}

// Visit tries to translate the node.
// If the method returns true, the visitor will not walk over AST nodes under v.
func (p *astProcessor) Visit(v any) bool {
	switch node := v.(type) {
	case *ast.Body:
		logging.LogForComponent("astProcessor").Debugf("Body: -> %+v", v)
		return p.translateQuery(*node)
	case *ast.Expr:
		logging.LogForComponent("astProcessor").Debugf("Expr: -> %+v", v)
		return p.translateExpr(node)
	case *ast.Term:
		logging.LogForComponent("astProcessor").Debugf("Term: -> %+v", v)
		return p.translateTerm(node)
	default:
		if p.skipUnknown || p.validateMode {
			logging.LogForComponent("astProcessor").Warnf("Unexpectedly visiting children of: %T -> %+v", v, v)
		}
		// If not skipping unknown ast nodes -> error
		if !p.skipUnknown {
			p.errors = append(p.errors, fmt.Sprintf("Unexpectedly visiting children of: %T -> %+v", v, v))
		}
	}
	return false
}

func (p *astProcessor) translateQuery(q ast.Body) bool {
	logging.LogForComponent("astProcessor").Debugf("================= PROCESS QUERY: %+v", q)
	for _, exp := range q {
		p.visitor.Walk(exp)
	}

	logging.LogForComponent("astProcessor").Debugf("%30sAppend to Conjunctions -> %+v", "", p.relations)
	p.conjunctions = append(p.conjunctions, p.relations...)

	logging.LogForComponent("astProcessor").Debugf("%30sClean entities and relations", "")
	p.entities = make(map[string]any)
	p.relations = p.relations[:0]

	return false
}

func (p *astProcessor) translateExpr(node *ast.Expr) bool {
	if !node.IsCall() {
		return true
	}

	op := data.Operator{Value: node.Operator().String()}
	p.operands.Push([]data.Node{})
	for _, term := range node.Operands() {
		p.visitor.Walk(term)
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
	p.entities = make(map[string]any)
	return true
}

func (p *astProcessor) translateTerm(node *ast.Term) bool {
	switch v := node.Value.(type) {
	case ast.Boolean:
		util.AppendToTopChecked("astProcessor", &p.operands, makeConstant(v.String()))
		return true
	case ast.String:
		util.AppendToTopChecked("astProcessor", &p.operands, makeConstant(v.String()))
		return true
	case ast.Number:
		util.AppendToTopChecked("astProcessor", &p.operands, makeConstant(v.String()))
		return true
	case ast.Ref:
		if len(v) == 3 {
			entity := data.Entity{Value: normalizeString(v[1].Value.String())}
			p.entities[entity.Value] = nil
			if p.fromEntity == nil {
				p.fromEntity = &entity
			}
			attribute := data.Attribute{Entity: entity, Name: normalizeString(v[2].Value.String())}
			util.AppendToTopChecked("astProcessor", &p.operands, data.Node(attribute))
		}
		return true
	case ast.Call:
		op := data.Operator{Value: v[0].String()}
		p.operands.Push([]data.Node{})
		for _, term := range v[1:] {
			p.visitor.Walk(term)
		}

		functionOperands, err := p.operands.Pop()
		if err != nil {
			logging.LogForComponent("astProcessor").Panicf("Error popping operands: %s", err)
		}
		util.AppendToTopChecked("astProcessor", &p.operands, data.Node(data.Call{
			Operator: op,
			Operands: functionOperands,
		}))
		return true
	default:
		if p.skipUnknown || p.validateMode {
			logging.LogForComponent("astProcessor").Warnf("Unexpected term Node: %T -> %+v", v, v)
		}
		// If not skipping unknown ast nodes -> error
		if !p.skipUnknown {
			p.errors = append(p.errors, fmt.Sprintf("Unexpected term Node: %T -> %+v", v, v))
		}
	}
	return false
}

func makeConstant(value string) data.Node {
	// Convert "<Const>" to <Const>
	value, err := unquote(value)
	if err != nil {
		logging.LogForComponent("astProcessor").Panicf("Error triming surrounding quotes: %s", err)
	}

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

func unquote(s string) (string, error) {
	if len(s) < 2 {
		return s, nil
	}

	if rune(s[0]) == '"' && rune(s[len(s)-1]) == '"' {
		return strconv.Unquote(s)
	}

	return s, nil
}

func keys(input map[string]any) []string {
	i := 0
	result := make([]string, len(input))
	for k := range input {
		result[i] = k
		i++
	}
	return result
}
