package data

import (
	"fmt"
	"github.com/Foundato/kelon/configs"
)

type Datastore interface {
	Configure(appConf *configs.AppConfig, alias string) error
	Execute(query *Node) (bool, error)
}

type Node interface {
	String() string
	Walk(func(v Node))
}

type CallOpMapper interface {
	Handles() string
	Map(args ...string) string
}

type Operator struct {
	Value string
}

type Constant struct {
	Value     string
	IsNumeric bool
	IsInt     bool
	IsFloat32 bool
}

type Entity struct {
	Value string
}

type Attribute struct {
	Entity Entity
	Name   string
}

type Call struct {
	Operator Operator
	Operands []Node
}

type Conjunction struct {
	Clauses []Node
}

type Disjunction struct {
	Clauses []Node
}

type Condition struct {
	Clause Node
}

type Link struct {
	Entities   []Entity
	Conditions []Node
}

type Query struct {
	From      Entity
	Link      Link
	Condition Condition
}

type Union struct {
	Clauses []Node
}

// Interface implementations

func (o Operator) String() string {
	return o.Value
}
func (o Operator) Walk(vis func(v Node)) {
	vis(o)
}

func (c Constant) String() string {
	return c.Value
}
func (c Constant) Walk(vis func(v Node)) {
	vis(c)
}

func (e Entity) String() string {
	return e.Value
}
func (e Entity) Walk(vis func(v Node)) {
	vis(e)
}

func (a Attribute) String() string {
	return fmt.Sprintf("att(%s.%s)", a.Entity.String(), a.Name)
}
func (a Attribute) Walk(vis func(v Node)) {
	a.Entity.Walk(vis)
	vis(a)
}

func (c Call) String() string {
	var operands []string
	for _, o := range c.Operands {
		operands = append(operands, o.String())
	}
	return fmt.Sprintf("%s(%+v)", c.Operator.String(), operands)
}
func (c Call) Walk(vis func(v Node)) {
	c.Operator.Walk(vis)
	for _, o := range c.Operands {
		o.Walk(vis)
	}
	vis(c)
}

func (c Conjunction) String() string {
	var clauses []string
	for _, o := range c.Clauses {
		clauses = append(clauses, o.String())
	}
	return fmt.Sprintf("conj(%+v)", clauses)
}
func (c Conjunction) Walk(vis func(v Node)) {
	for _, o := range c.Clauses {
		o.Walk(vis)
	}
	vis(c)
}

func (d Disjunction) String() string {
	var relations []string
	for _, o := range d.Clauses {
		relations = append(relations, o.String())
	}
	return fmt.Sprintf("disj(%+v)", relations)
}
func (d Disjunction) Walk(vis func(v Node)) {
	for _, o := range d.Clauses {
		o.Walk(vis)
	}
	vis(d)
}

func (c Condition) String() string {
	return fmt.Sprintf("cond(%s)", c.Clause)
}
func (c Condition) Walk(vis func(v Node)) {
	c.Clause.Walk(vis)
	vis(c)
}

func (l Link) String() string {
	var links []string
	for i, e := range l.Entities {
		links = append(links, fmt.Sprintf("(%s ON %s)", e, l.Conditions[i]))
	}
	return fmt.Sprintf("link(%+v)", links)
}
func (l Link) Walk(vis func(v Node)) {
	for _, c := range l.Conditions {
		c.Walk(vis)
	}
	for _, e := range l.Entities {
		e.Walk(vis)
	}
	vis(l)
}

func (q Query) String() string {
	return fmt.Sprintf("query(%s, %+v, %s)", q.From.String(), q.Link, q.Condition.String())
}
func (q Query) Walk(vis func(v Node)) {
	q.Link.Walk(vis)
	q.Condition.Walk(vis)
	q.From.Walk(vis)
	vis(q)
}

func (u Union) String() string {
	var clauses []string
	for _, o := range u.Clauses {
		clauses = append(clauses, o.String())
	}
	return fmt.Sprintf("union(%+v)", clauses)
}
func (u Union) Walk(vis func(v Node)) {
	for _, c := range u.Clauses {
		c.Walk(vis)
	}
	vis(u)
}
