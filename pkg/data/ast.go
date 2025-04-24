package data

import "fmt"

// Node is the abstract interface that every Node of the Query-AST implements.
type Node interface {

	// String returns the current node as string representation (In case of a leave this will be the contained Value).
	String() string

	// Walk the current node (Buttom-Up, Left-to-Right).
	Walk(func(v Node) error) error
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

// Condition is a single root condition.
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

// Entity represents an entity
type Entity struct {
	Value string
}

// An Operator of the AST.
type Operator struct {
	Value string
}

// Constant is a simple constant.
type Constant struct {
	Value     string
	IsNumeric bool
	IsInt     bool
	IsFloat32 bool
}

// Interface implementations

// String Implements data.Node
func (o Operator) String() string {
	return o.Value
}

// Walk Implements data.Node
func (o Operator) Walk(vis func(v Node) error) error {
	return vis(o)
}

// String Implements data.Node
func (c Constant) String() string {
	return c.Value
}

// Walk Implements data.Node
func (c Constant) Walk(vis func(v Node) error) error {
	return vis(c)
}

// String Implements data.Node
func (e Entity) String() string {
	return e.Value
}

// Walk Implements data.Node
func (e Entity) Walk(vis func(v Node) error) error {
	return vis(e)
}

// String Implements data.Node
func (a Attribute) String() string {
	return fmt.Sprintf("att(%s.%s)", a.Entity.String(), a.Name)
}

// Walk Implements data.Node
func (a Attribute) Walk(vis func(v Node) error) error {
	if err := a.Entity.Walk(vis); err != nil {
		return err
	}
	return vis(a)
}

// String Implements data.Node
func (c Call) String() string {
	operands := make([]string, len(c.Operands))
	for i, o := range c.Operands {
		operands[i] = o.String()
	}

	return fmt.Sprintf("%s(%+v)", c.Operator.String(), operands)
}

// Walk Implements data.Node
func (c Call) Walk(vis func(v Node) error) error {
	if err := c.Operator.Walk(vis); err != nil {
		return err
	}
	for _, o := range c.Operands {
		if err := o.Walk(vis); err != nil {
			return err
		}
	}
	return vis(c)
}

// String Implements data.Node
func (c Conjunction) String() string {
	clauses := make([]string, len(c.Clauses))
	for i, o := range c.Clauses {
		clauses[i] = o.String()
	}
	return fmt.Sprintf("conj(%+v)", clauses)
}

// Walk Implements data.Node
func (c Conjunction) Walk(vis func(v Node) error) error {
	for _, o := range c.Clauses {
		if err := o.Walk(vis); err != nil {
			return err
		}
	}
	return vis(c)
}

// String Implements data.Node
func (d Disjunction) String() string {
	relations := make([]string, len(d.Clauses))
	for i, o := range d.Clauses {
		relations[i] = o.String()
	}
	return fmt.Sprintf("disj(%+v)", relations)
}

// Walk Implements data.Node
func (d Disjunction) Walk(vis func(v Node) error) error {
	for _, o := range d.Clauses {
		if err := o.Walk(vis); err != nil {
			return err
		}
	}
	return vis(d)
}

// Implements data.Node
func (c Condition) String() string {
	return fmt.Sprintf("cond(%s)", c.Clause)
}

// Walk Implements data.Node
func (c Condition) Walk(vis func(v Node) error) error {
	if err := c.Clause.Walk(vis); err != nil {
		return err
	}
	return vis(c)
}

// String Implements data.Node
func (l Link) String() string {
	links := make([]string, len(l.Entities))
	for i, e := range l.Entities {
		links[i] = fmt.Sprintf("%s, ", e)
	}
	return fmt.Sprintf("link(%+v)", links)
}

// Walk Implements data.Node
func (l Link) Walk(vis func(v Node) error) error {
	for _, e := range l.Entities {
		if err := e.Walk(vis); err != nil {
			return err
		}
	}
	return vis(l)
}

// String Implements data.Node
func (q Query) String() string {
	return fmt.Sprintf("query(%s, %+v, %s)", q.From.String(), q.Link, q.Condition.String())
}

// Walk Implements data.Node
func (q Query) Walk(vis func(v Node) error) error {
	if err := q.Link.Walk(vis); err != nil {
		return err
	}
	if err := q.Condition.Walk(vis); err != nil {
		return err
	}
	if err := q.From.Walk(vis); err != nil {
		return err
	}
	return vis(q)
}

// String implements data.Node
func (u Union) String() string {
	clauses := make([]string, len(u.Clauses))
	for i, o := range u.Clauses {
		clauses[i] = o.String()
	}
	return fmt.Sprintf("union(%+v)", clauses)
}

// Walk implements data.Node
func (u Union) Walk(vis func(v Node) error) error {
	for _, c := range u.Clauses {
		if err := c.Walk(vis); err != nil {
			return err
		}
	}
	return vis(u)
}
