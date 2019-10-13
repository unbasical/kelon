package translate

import (
	"fmt"
	"github.com/open-policy-agent/opa/ast"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type astPreprocessor struct {
	tableNames        map[string]string
	tableVars         map[string][]*ast.Term
	expectedDatastore string
}

func (processor *astPreprocessor) Process(queries []ast.Body, datastore string) ([]ast.Body, error) {
	var transformedQueries []ast.Body
	processor.expectedDatastore = fmt.Sprintf("\"%s\"", datastore)

	for _, q := range queries {
		log.Debugf("================= PREPROCESS QUERY: %+v\n", q)
		processor.tableNames = make(map[string]string)
		processor.tableVars = make(map[string][]*ast.Term)

		var transformedExprs []*ast.Expr
		for _, expr := range q {
			// Only transform operands
			terms := []*ast.Term{ast.NewTerm(expr.Operator())}
			for _, o := range expr.Operands() {
				trans, err := processor.transformRefs(o)
				if err != nil {
					return nil, errors.Wrapf(err, "Preprocessor: Error while preprocessing Operator [%+v] of expression [%+v]", o, expr)
				}
				terms = append(terms, ast.NewTerm(trans.(ast.Value)))
			}
			transformedExprs = append(transformedExprs, ast.NewExpr(terms))
		}
		transformedQueries = append(transformedQueries, ast.NewBody(transformedExprs...))
	}
	return transformedQueries, nil
}

func (processor *astPreprocessor) transformRefs(value interface{}) (interface{}, error) {

	trans := func(node ast.Ref) (ast.Value, error) {
		// Skip scalars (TODO: check there is a more elegant way to do this)
		if len(node) == 1 {
			return node, nil
		}

		head := node[0].Value.String()
		if match, ok := processor.tableVars[head]; ok {
			// Expand ref in case head was an intermediate var. E.g.,
			// "data.foo[x]; x.bar" => "data.foo[x]; data.foo.bar".
			return ast.Ref{}.Concat(append(match, node[1:]...)), nil
		}

		// Validate if datastore prefix is present
		if head == "data" && node[1].String() != processor.expectedDatastore {
			return nil, errors.Errorf("Invalid reference: expected [data.%s.<table>] but found reference [%s] \n", processor.expectedDatastore, node.String())
		}

		rowId := node[3].Value

		// Refs must be of the form data.<datastore>.<table>[<iterator>].<column>.
		if _, ok := rowId.(ast.Var); !ok {
			return nil, errors.Errorf("Invalid reference: row identifier type not supported: %s\n", rowId.String())
		}

		// Remove datastore from prefix
		prefix := []*ast.Term{node[0], node[2]}

		// Add mapping so that we can expand refs above.
		processor.tableVars[rowId.String()] = prefix
		tableName := node[2].Value.String()

		// Keep track of iterators used for each table. We do not support
		// self-links currently. Self-links require namespacing in the SQL
		// value.
		if _, ok := processor.tableNames[tableName]; ok {
			return nil, errors.New("invalid reference: self-links not supported")
		} else {
			processor.tableNames[tableName] = rowId.String()
		}

		// Rewrite ref to remove iterator var. E.g., "data.<datastore>.foo[x].bar" =>
		// "data.foo.bar".
		return ast.Ref{}.Concat(append(prefix, node[4:]...)), nil
	}

	return ast.TransformRefs(value, trans)
}