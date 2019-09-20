package data

import (
	"fmt"
	"github.com/Foundato/kelon/configs"
	"github.com/pkg/errors"
	"log"
	"strings"
)

type Datastore interface {
	Configure(appConf *configs.AppConfig) error
	Execute(query *Node) (*[]interface{}, error)
}

type postgres struct {
	appConf    *configs.AppConfig
	configured bool
}

var operators = map[string]string{
	// Relation operators
	"eq":    "=",
	"equal": "=",
	"neq":   "!=",
	"lt":    "<",
	"gt":    ">",
	"lte":   "<=",
	"gte":   ">=",
	// Call operators
	"abs": "abs",
}

func NewPostgresDatastore() Datastore {
	return &postgres{
		appConf:    nil,
		configured: false,
	}
}

func (ds *postgres) Configure(appConf *configs.AppConfig) error {
	if appConf == nil {
		return errors.New("PostgresDatastore: AppConfig not configured! ")
	}
	ds.appConf = appConf
	ds.configured = true
	log.Println("Configured PostgresDatastore")
	return nil
}

func (ds postgres) Execute(query *Node) (*[]interface{}, error) {
	if !ds.configured {
		return nil, errors.New("PostgresDatastore was not configured! Please call Configure(). ")
	}

	// Translate query to into sql statement
	sql, err := ds.translate(query)
	if err != nil {
		return nil, err
	}
	println(sql)

	// TODO implement Datastore access
	var stub []interface{}
	stub = append(stub, "Stub")
	return &stub, nil
}

func (ds postgres) translate(query *Node) (string, error) {
	fmt.Printf("Datastore -> Processing: %s\n", (*query).String())

	var sql stack
	var selects stack
	var entities stack
	var relations stack
	var joins stack

	// Walk query
	(*query).Walk(func(q Node) {
		switch v := q.(type) {
		case Union:
			// Expected stack:  top -> [Queries...]
			sql = sql.Push(strings.Join(selects, "\nUNION\n"))
			selects = selects[:0]
		case Query:
			// Expected stack:  top -> [entity]
			var (
				entity     string
				joinClause string
			)
			entities, entity = entities.Pop()
			for _, j := range joins {
				joinClause += j
			}

			selects = selects.Push(fmt.Sprintf("SELECT count(*) FROM %s%s%s", entity, joinClause, relations[0]))
			joins = joins[:0]
			relations = relations[:0]
		case Link:
			// Expected stack:  top -> []
			for i, entity := range entities {
				joins = joins.Push(fmt.Sprintf(" INNER JOIN %s ON %s", entity, strings.Replace(relations[i], "WHERE", "", 1)))
			}
			entities = entities[:0]
			relations = relations[:0]
		case Condition:
			// Expected stack:  top -> []
			relations = relations[:0].Push(fmt.Sprintf(" WHERE %s", relations[0]))
		case Disjunction:
			// Expected stack:  top -> []
			relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(sql, " OR ")))
		case Conjunction:
			// Expected stack:  relations-top -> []
			relations = relations[:0].Push(fmt.Sprintf("(%s)", strings.Join(relations, " AND ")))
		case Relation:
			// Expected stack:  top -> [lhs, op, rhs]
			relations = relations.Push(fmt.Sprintf("%s %s %s", sql[2], sql[1], sql[0]))
			sql = sql[:0]
		case Attribute:
			// Expected stack:  top -> [...]
			var entity string
			entities, entity = entities.Pop()
			sql = sql.Push(fmt.Sprintf("%s.%s", entity, v.Name))
		case Call:
			// Expected stack:  top -> [call-op, args...]
			top := len(sql) - 1
			sql = sql[:0].Push(fmt.Sprintf("%s (%s)", sql[top], strings.Join(sql[:top], ", ")))
		case Operator:
			sqlOp, ok := operators[v.String()]
			if !ok {
				panic(fmt.Sprintf("Datastore: Call-Operator [%s] is not supported!", v.String()))
			}
			sql = sql.Push(sqlOp)
		case Entity:
			entities = entities.Push(v.String())
		case Constant:
			sql = sql.Push(v.String())
		default:
			fmt.Printf("Postgres datastore: Unexpected query: %T -> %+v\n", v, v)
		}
	})

	// TODO Execute query in database
	println("EXECUTING STATEMENT ==================")
	for _, s := range sql {
		println(s)
	}
	println("END ==================")
	return "", nil
}

type stack []string

func (s stack) Empty() bool {
	return len(s) == 0
}

func (s stack) Push(v string) stack {
	return append(s, v)
}

func (s stack) Pop() (stack, string) {
	if l := len(s); l > 0 {
		return s[:l-1], s[l-1]
	} else {
		panic("Stack is empty!")
	}
}
