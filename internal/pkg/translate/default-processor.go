package translate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Foundato/kelon/pkg/constants/logging"
	"github.com/Foundato/kelon/pkg/data"
	"github.com/open-policy-agent/opa/ast"
)

type astProcessor struct {
	fromEntity   *data.Entity
	link         map[string]interface{}
	conjunctions []data.Node
	entities     map[string]interface{}
	relations    []data.Node
	operands     NodeStack
}

func newAstProcessor() *astProcessor {
	return &astProcessor{}
}

// See translate.AstTranslator.
func (p *astProcessor) Process(queries []ast.Body) (*data.Node, error) {
	p.link = make(map[string]interface{})
	p.conjunctions = []data.Node{}
	p.entities = make(map[string]interface{})
	p.relations = []data.Node{}
	p.operands = [][]data.Node{}

	// NEW ERA
	clauses := make([]data.Node, len(queries))
	for i, q := range queries {
		p.translateQuery(q)
		condition := data.Condition{Clause: data.Conjunction{Clauses: append(p.conjunctions[:0:0], p.conjunctions...)}}

		// Add new Query
		delete(p.link, p.fromEntity.String())
		clauses[i] = data.Query{
			From:      *p.fromEntity,
			Link:      toDataLink(p.link),
			Condition: condition,
		}

		// Cleanup
		p.conjunctions = p.conjunctions[:0]
		p.fromEntity = nil
		p.link = make(map[string]interface{})
	}

	var result data.Node = data.Union{Clauses: clauses}
	return &result, nil
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
		logging.LogForComponent("astProcessor").Warnf("Unexpectedly visiting children of: %T -> %+v", v, v)
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
	p.operands = p.operands.Push([]data.Node{})
	for _, term := range node.Operands() {
		ast.Walk(p, term)
	}

	var functionOperands []data.Node
	p.operands, functionOperands = p.operands.Pop()
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
		p.operands.AppendToTop(makeConstant(v.String()))
		return nil
	case ast.String:
		p.operands.AppendToTop(makeConstant(v.String()))
		return nil
	case ast.Number:
		p.operands.AppendToTop(makeConstant(v.String()))
		return nil
	case ast.Ref:
		if len(v) == 3 {
			entity := data.Entity{Value: normalizeString(v[1].Value.String())}
			p.entities[entity.Value] = nil
			if p.fromEntity == nil {
				p.fromEntity = &entity
			}
			attribute := data.Attribute{Entity: entity, Name: normalizeString(v[2].Value.String())}
			p.operands.AppendToTop(attribute)
		}
		return nil
	case ast.Call:
		op := data.Operator{Value: v[0].String()}
		p.operands = p.operands.Push([]data.Node{})
		for _, term := range v[1:] {
			ast.Walk(p, term)
		}

		var functionOperands []data.Node
		p.operands, functionOperands = p.operands.Pop()
		p.operands.AppendToTop(data.Call{
			Operator: op,
			Operands: functionOperands,
		})
		return nil
	default:
		logging.LogForComponent("astProcessor").Warnf("Unexpected term Node: %T -> %+v", v, v)
	}
	return p
}

func makeConstant(value string) *data.Constant {
	// Convert "<Const>" to <Const>
	value = strings.TrimFunc(value, func(r rune) bool {
		return r == '"'
	})

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

func keys(input map[string]interface{}) []string {
	i := 0
	result := make([]string, len(input))
	for k := range input {
		result[i] = k
		i++
	}
	return result
}

// Stack which is used for AST-transformation
type NodeStack [][]data.Node

// Push new element to stack
func (s NodeStack) Push(v []data.Node) NodeStack {
	logging.LogForComponent("NodeStack").Debugf("%30sOperands len(%d) PUSH(%+v)", "", len(s), v)
	return append(s, v)
}

// Append new node to top element of the stack
func (s NodeStack) AppendToTop(v data.Node) {
	l := len(s)
	if l <= 0 {
		logging.LogForComponent("NodeStack").Panic("Stack is empty!")
	}

	s[l-1] = append(s[l-1], v)
	logging.LogForComponent("NodeStack").Debugf("%30sOperands len(%d) APPEND |%+v <- TOP", "", len(s), s[l-1])
}

// Pop top element from Stack
func (s NodeStack) Pop() (NodeStack, []data.Node) {
	l := len(s)
	if l <= 0 {
		logging.LogForComponent("NodeStack").Panic("Stack is empty!")
	}

	logging.LogForComponent("NodeStack").Debugf("%30sOperands len(%d) POP()", "", len(s))
	return s[:l-1], s[l-1]
}
