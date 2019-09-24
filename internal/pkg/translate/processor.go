package translate

import (
	"fmt"
	"github.com/Foundato/kelon/internal/pkg/data"
	"github.com/open-policy-agent/opa/ast"
	"log"
	"strconv"
	"strings"
)

type AstProcessor struct {
	fromEntity   *data.Entity
	link         data.Link
	conjunctions [][]data.Node
	entities     map[string]interface{}
	relations    []data.Node
	operands     stack
	clauses      []data.Node
}

func (p *AstProcessor) Process(queries []ast.Body) (*data.Node, error) {
	p.link = data.Link{}
	p.conjunctions = [][]data.Node{}
	p.entities = make(map[string]interface{})
	p.relations = []data.Node{}
	p.operands = [][]data.Node{}
	p.clauses = []data.Node{}

	for _, q := range queries {
		p.translateQuery(q)
	}

	var result data.Node
	result = data.Union{Clauses: p.clauses}
	return &result, nil
}

func (p *AstProcessor) translateQuery(q ast.Body) {
	ast.WalkExprs(q, p.translateExpr)

	// Append new Query
	var condition data.Condition
	if len(p.conjunctions) > 0 {
		var relations []data.Node
		for _, conj := range p.conjunctions {
			for _, rel := range conj {
				relations = append(relations, rel)
			}
		}

		condition = data.Condition{Clause: data.Conjunction{Clauses: relations}}
	}
	p.clauses = append(p.clauses, data.Query{
		From:      *p.fromEntity,
		Link:      p.link,
		Condition: condition,
	})

	// Cleanup
	p.conjunctions = p.conjunctions[:0]
	p.fromEntity = nil
	p.link = data.Link{}
}

// Pushes an element onto the relation stack.
func (p *AstProcessor) translateExpr(expr *ast.Expr) bool {
	if !expr.IsCall() {
		return false
	}
	if len(expr.Operands()) != 2 {
		panic("AstTranslator: Processor: Invalid expression: too many arguments")
	}

	// Translate operands
	p.operands = p.operands.Push([]data.Node{})
	for _, op := range expr.Operands() {
		p.translateTerm(op)
	}

	var operands []data.Node
	p.operands, operands = p.operands.Pop()
	op := data.Operator{Value: expr.Operator().String()}
	p.relations = append(p.relations, data.Relation{Operator: op, Lhs: operands[0], Rhs: operands[1]})

	// Test
	clonedRelations := append(p.relations[:0:0], p.relations...)
	if len(p.entities) > 1 {
		delete(p.entities, p.fromEntity.String())
		for _, linkedEntity := range p.link.Entities {
			delete(p.entities, linkedEntity.String())
		}

		entities := makeEntities(p.entities)
		if len(entities) != 1 {
			panic("Expected only one entity for this link")
		}
		p.link.Entities = append(p.link.Entities, entities[0])
		p.link.Conditions = append(p.link.Conditions, clonedRelations...)
	} else {
		p.conjunctions = append(p.conjunctions, clonedRelations)
	}
	// Cleanup
	p.entities = make(map[string]interface{})
	p.relations = p.relations[:0]
	return true
}

// Pushes an element onto the operand stack.
func (p *AstProcessor) translateTerm(term *ast.Term) bool {
	switch v := term.Value.(type) {
	case ast.Var:
		p.operands.AppendToTop(data.Constant{Value: v.String()})
		return true
	case ast.String:
		p.operands.AppendToTop(data.Constant{Value: tryCastToNumericString(v.String())})
		return true
	case ast.Number:
		p.operands.AppendToTop(data.Constant{Value: tryCastToNumericString(v.String())})
		return true
	case ast.Ref:
		if len(v) == 3 {
			entity := data.Entity{Value: normalizeString(v[1].Value.String())}
			if p.fromEntity == nil {
				p.fromEntity = &entity
			}
			p.entities[entity.Value] = nil
			attribute := data.Attribute{Entity: entity, Name: normalizeString(v[2].Value.String())}
			p.operands.AppendToTop(attribute)
			return true
		}
		return false
	case ast.Call:
		op := data.Operator{Value: v.String()}
		p.operands = p.operands.Push([]data.Node{})
		ast.WalkTerms(v, p.translateTerm)

		tmpStack, operands := p.operands.Pop()
		tmpStack.AppendToTop(data.Call{Operator: op, Operands: operands})
		p.operands = tmpStack
		return true

	default:
		log.Printf("Unexpected term Node: %T -> %+v\n", v, v)
		return false
	}
}

func makeEntities(set map[string]interface{}) []data.Entity {
	var entities []data.Entity
	for k := range set {
		entities = append(entities, data.Entity{Value: k})
	}
	return entities
}

type stack [][]data.Node

func (s stack) Push(v []data.Node) stack {
	return append(s, v)
}

func (s stack) AppendToTop(v data.Node) {
	if l := len(s); l > 0 {
		s[l-1] = append(s[l-1], v)
	} else {
		panic("Stack is empty!")
	}
}

func (s stack) Pop() (stack, []data.Node) {
	if l := len(s); l > 0 {
		return s[:l-1], s[l-1]
	} else {
		panic("Stack is empty!")
	}
}

func tryCastToNumericString(value string) string {
	if num, err := strconv.Atoi(value); err != nil {
		return fmt.Sprintf("%d", num)
	}
	if num, err := strconv.ParseFloat(value, 32); err != nil {
		return fmt.Sprintf("%f", num)
	}
	return value
}

func normalizeString(value string) string {
	return strings.ReplaceAll(value, "\"", "")
}
