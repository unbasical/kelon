package data

import "fmt"

// Node is the abstract interface that every Node of the Query-AST implements.
type Node interface {

	// Get the current node as string representation (In case of a leave this will be the contained Value).
	String() string

	// Walk the current node (Buttom-Up, Left-to-Right).
	Walk(func(v Node))
}

// Union of multiple queries.
type Union struct {
	Clauses []Node
}

// Query which contains links between entities and a condition.
type Query struct {
	From      Entity
	Link      Link
	Condition Condition
}

// Link between a parent entity and a list of entities with corresponding conditions.
type Link struct {
	Entities []Entity
}

// A single root condition.
type Condition struct {
	Clause Node
}

// Conjunction of several conditions.
type Conjunction struct {
	Clauses []Node
}

// Disjunction of several disjunctions.
type Disjunction struct {
	Clauses []Node
}

// Call represented by an operand and a list of arguments.
type Call struct {
	Operator Operator
	Operands []Node
}

// Attribute of an entity.
type Attribute struct {
	Entity Entity
	Name   string
}

// An Entity
type Entity struct {
	Value string
}

// An Operator of the AST.
type Operator struct {
	Value string
}

// A simple Constant.
type Constant struct {
	Value     string
	IsNumeric bool
	IsInt     bool
	IsFloat32 bool
}

// Interface implementations

// Implements data.Node
func (o Operator) String() string {
	return o.Value
}

// Implements data.Node
func (o Operator) Walk(vis func(v Node)) {
	vis(o)
}

// Implements data.Node
func (c Constant) String() string {
	return c.Value
}

// Implements data.Node
func (c Constant) Walk(vis func(v Node)) {
	vis(c)
}

// Implements data.Node
func (e Entity) String() string {
	return e.Value
}

// Implements data.Node
func (e Entity) Walk(vis func(v Node)) {
	vis(e)
}

// Implements data.Node
func (a Attribute) String() string {
	return fmt.Sprintf("att(%s.%s)", a.Entity.String(), a.Name)
}

// Implements data.Node
func (a Attribute) Walk(vis func(v Node)) {
	a.Entity.Walk(vis)
	vis(a)
}

// Implements data.Node
func (c Call) String() string {
	operands := make([]string, len(c.Operands))
	for i, o := range c.Operands {
		operands[i] = o.String()
	}

	return fmt.Sprintf("%s(%+v)", c.Operator.String(), operands)
}

// Implements data.Node
func (c Call) Walk(vis func(v Node)) {
	c.Operator.Walk(vis)
	for _, o := range c.Operands {
		o.Walk(vis)
	}
	vis(c)
}

// Implements data.Node
func (c Conjunction) String() string {
	clauses := make([]string, len(c.Clauses))
	for i, o := range c.Clauses {
		clauses[i] = o.String()
	}
	return fmt.Sprintf("conj(%+v)", clauses)
}

// Implements data.Node
func (c Conjunction) Walk(vis func(v Node)) {
	for _, o := range c.Clauses {
		o.Walk(vis)
	}
	vis(c)
}

// Implements data.Node
func (d Disjunction) String() string {
	relations := make([]string, len(d.Clauses))
	for i, o := range d.Clauses {
		relations[i] = o.String()
	}
	return fmt.Sprintf("disj(%+v)", relations)
}

// Implements data.Node
func (d Disjunction) Walk(vis func(v Node)) {
	for _, o := range d.Clauses {
		o.Walk(vis)
	}
	vis(d)
}

// Implements data.Node
func (c Condition) String() string {
	return fmt.Sprintf("cond(%s)", c.Clause)
}

// Implements data.Node
func (c Condition) Walk(vis func(v Node)) {
	c.Clause.Walk(vis)
	vis(c)
}

// Implements data.Node
func (l Link) String() string {
	links := make([]string, len(l.Entities))
	for i, e := range l.Entities {
		links[i] = fmt.Sprintf("%s, ", e)
	}
	return fmt.Sprintf("link(%+v)", links)
}

// Implements data.Node
func (l Link) Walk(vis func(v Node)) {
	for _, e := range l.Entities {
		e.Walk(vis)
	}
	vis(l)
}

// Implements data.Node
func (q Query) String() string {
	return fmt.Sprintf("query(%s, %+v, %s)", q.From.String(), q.Link, q.Condition.String())
}

// Implements data.Node
func (q Query) Walk(vis func(v Node)) {
	q.Link.Walk(vis)
	q.Condition.Walk(vis)
	q.From.Walk(vis)
	vis(q)
}

// Implements data.Node
func (u Union) String() string {
	clauses := make([]string, len(u.Clauses))
	for i, o := range u.Clauses {
		clauses[i] = o.String()
	}
	return fmt.Sprintf("union(%+v)", clauses)
}

// Implements data.Node
func (u Union) Walk(vis func(v Node)) {
	for _, c := range u.Clauses {
		c.Walk(vis)
	}
	vis(u)
}
