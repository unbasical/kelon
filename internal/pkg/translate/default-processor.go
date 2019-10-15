package translate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/open-policy-agent/opa/ast"
	log "github.com/sirupsen/logrus"
)

type astProcessor struct {
	fromEntity   *data.Entity
	link         data.Link
	conjunctions []data.Node
	entities     map[string]interface{}
	relations    []data.Node
	operands     NodeStack
}

var inset string = "\t\t\t\t\t"

// See translate.AstTranslator.
func (p *astProcessor) Process(queries []ast.Body) (*data.Node, error) {
	p.link = data.Link{}
	p.conjunctions = []data.Node{}
	p.entities = make(map[string]interface{})
	p.relations = []data.Node{}
	p.operands = [][]data.Node{}

	// NEW ERA
	var clauses []data.Node
	for _, q := range queries {
		p.translateQuery(q)
		condition := data.Condition{Clause: data.Conjunction{Clauses: append(p.conjunctions[:0:0], p.conjunctions...)}}

		// Add new Query
		clauses = append(clauses, data.Query{
			From:      *p.fromEntity,
			Link:      p.link,
			Condition: condition,
		})

		// Cleanup
		p.conjunctions = p.conjunctions[:0]
		p.fromEntity = nil
		p.link = data.Link{}
	}

	var result data.Node
	result = data.Union{Clauses: clauses}
	return &result, nil
}

// Implementation of the visitor pattern to crawl the AST.
func (p *astProcessor) Visit(v interface{}) ast.Visitor {
	switch node := v.(type) {
	case *ast.Body:
		log.Debugf("Body: -> %+v\n", v)
		return p.translateQuery(*node)
	case *ast.Expr:
		log.Debugf("Expr: -> %+v\n", v)
		return p.translateExpr(*node)
	case *ast.Term:
		log.Debugf("Term: -> %+v\n", v)
		return p.translateTerm(*node)
	default:
		log.Warnf("Unexpectedly visiting children of: %T -> %+v\n", v, v)
	}
	return p
}

func (p *astProcessor) translateQuery(q ast.Body) ast.Visitor {
	log.Debugf("================= PROCESS QUERY: %+v\n", q)
	for _, exp := range q {
		ast.Walk(p, exp)
	}

	log.Debugf("%sAppend to Conjunctions -> %+v\n", inset, p.relations)
	p.conjunctions = append(p.conjunctions, p.relations...)

	log.Debugf("%sClean entities and relations", inset)
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
		p.removeAlreadyJoinedEntities()
		if len(p.entities) > 1 {
			panic("Multi join not supported!")
		}

		joinEntity := data.Entity{Value: keys(p.entities)[0]}

		p.link.Entities = append(p.link.Entities, joinEntity)
		p.link.Conditions = append(p.link.Conditions, data.Call{
			Operator: op,
			Operands: functionOperands,
		})
		log.Debugf("%sLink: %+v\n", inset, p.link)
	} else {
		// Append new relation for conjunction
		p.relations = append(p.relations, data.Call{
			Operator: op,
			Operands: functionOperands,
		})
		log.Debugf("%sRelations: %+v\n", inset, p.relations)
	}

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
		log.Warnf("Unexpected term Node: %T -> %+v\n", v, v)
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

func (p *astProcessor) removeAlreadyJoinedEntities() {
	delete(p.entities, p.fromEntity.String())
	for _, e := range p.link.Entities {
		delete(p.entities, e.Value)
	}
}

func keys(input map[string]interface{}) []string {
	var result []string
	for k := range input {
		result = append(result, k)
	}
	return result
}

func (p astProcessor) isAlreadyLinked(entity data.Entity) bool {
	for _, e := range p.link.Entities {
		if e.Value == entity.Value {
			return false
		}
	}
	return true
}

// Stack which is used for AST-transformation
type NodeStack [][]data.Node

// Push new element to stack
func (s NodeStack) Push(v []data.Node) NodeStack {
	log.Debugf("%sOperands len(%d) PUSH(%+v)\n", inset, len(s), v)
	return append(s, v)
}

// Append new node to top element of the stack
func (s NodeStack) AppendToTop(v data.Node) {
	if l := len(s); l > 0 {
		s[l-1] = append(s[l-1], v)
		log.Debugf("%sOperands len(%d) APPEND |%+v <- TOP\n", inset, len(s), s[l-1])
	} else {
		panic("Stack is empty!")
	}
}

// Pop top element from Stack
func (s NodeStack) Pop() (NodeStack, []data.Node) {
	if l := len(s); l > 0 {
		log.Debugf("%sOperands len(%d) POP()\n", inset, len(s))
		return s[:l-1], s[l-1]
	} else {
		panic("Stack is empty!")
	}
}
