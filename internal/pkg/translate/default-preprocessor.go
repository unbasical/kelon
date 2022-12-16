package translate

import (
	"context"
	"strconv"

	"github.com/open-policy-agent/opa/ast"
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/constants/logging"
	"golang.org/x/exp/slices"
)

type preprocessedQuery struct {
	query     ast.Body
	datastore string
}

type astPreprocessor struct {
	tableNames        map[string]string
	tableVars         map[string][]*ast.Term
	localVars         map[string]*ast.Term
	datastorePool     []string
	expectedDatastore string
}

func newAstPreprocessor() *astPreprocessor {
	return &astPreprocessor{}
}

// Process the AST to simplify the translation process.
// Refs are rewritten to correspond directly to Entities and Attributes.
//
// Refs are rewritten to correspond directly to SQL tables and columns.
// Specifically, refs of the form data.foo[var].bar are rewritten as data.foo.bar. Similarly, if var is
// dereferenced later in the query, e.g., var.baz, that will be rewritten as data.foo.baz.
func (processor *astPreprocessor) Process(ctx context.Context, queries []ast.Body, datastores []string) ([]preprocessedQuery, error) {
	transformedQueries := make([]preprocessedQuery, len(queries))
	processor.datastorePool = datastores

	for i, q := range queries {
		logging.LogForComponent("astPreprocessor").Debugf("================= PREPROCESS QUERY: %+v", q)
		processor.tableNames = make(map[string]string)
		processor.tableVars = make(map[string][]*ast.Term)
		processor.localVars = make(map[string]*ast.Term)
		processor.expectedDatastore = ""

		var transformedExprs []*ast.Expr
		for _, expr := range q {
			// Only transform operands
			terms := []*ast.Term{ast.NewTerm(expr.Operator())}
			for _, o := range expr.Operands() {
				trans, err := processor.transformRefs(o)
				if err != nil {
					return nil, errors.Wrapf(err, "Preprocessor: Error while preprocessing Operator %T -> [%+v] of expression [%+v]", o, o, expr)
				}
				terms = append(terms, ast.NewTerm(trans.(ast.Value)))
			}

			terms, err := processor.substituteVars(terms)
			if err != nil {
				return []preprocessedQuery{}, errors.Wrapf(err, "Preprocessor: Error while preprocessing Expression [%+v]", expr)
			}

			if terms != nil {
				transformedExprs = append(transformedExprs, ast.NewExpr(terms))
			}
		}
		transformedQueries[i] = preprocessedQuery{query: ast.NewBody(transformedExprs...), datastore: processor.expectedDatastore}
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

		dsNode, err := strconv.Unquote(node[1].String())
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to unquote")
		}

		// if no datastore was configured yet, set one
		if processor.expectedDatastore == "" {
			if slices.Contains(processor.datastorePool, dsNode) {
				processor.expectedDatastore = dsNode
			} else {
				return nil, errors.Errorf("Invalid reference: expected one of %+v, but got [%s]", processor.datastorePool, node[1].String())
			}
		}

		// Validate if datastore prefix is present
		if head == "data" && dsNode != processor.expectedDatastore {
			return nil, errors.Errorf("Invalid reference: expected [data.%s.<table>] but found reference [%s] ", processor.expectedDatastore, node.String())
		}

		rowID := node[3].Value

		// Refs must be of the form data.<datastore>.<table>[<iterator>].<column>.
		if _, ok := rowID.(ast.Var); !ok {
			return nil, errors.Errorf("Invalid reference: row identifier type not supported: %s", rowID.String())
		}

		// Remove datastore from prefix
		prefix := []*ast.Term{node[0], node[2]}

		// Add mapping so that we can expand refs above.
		processor.tableVars[rowID.String()] = prefix
		tableName := node[2].Value.String()

		// Keep track of iterators used for each table. We do not support
		// self-links currently. Self-links require namespacing in the SQL
		// value.
		if match, ok := processor.tableNames[tableName]; ok && match != rowID.String() {
			return nil, errors.Errorf("invalid reference: self-links not supported")
		}
		processor.tableNames[tableName] = rowID.String()

		// Rewrite ref to remove iterator var. E.g., "data.<datastore>.foo[x].bar" =>
		// "data.foo.bar".
		return ast.Ref{}.Concat(append(prefix, node[4:]...)), nil
	}

	return ast.TransformRefs(value, trans)
}

func (processor *astPreprocessor) substituteVars(terms []*ast.Term) ([]*ast.Term, error) {
	// local variable declaration -> store and return
	if isLocalVarDeclaration(terms) {
		v, _ := terms[1].Value.(ast.Var)
		processor.localVars[v.String()] = terms[2]

		return nil, nil
	}

	var transformedTerms []*ast.Term
	for _, term := range terms {
		v, ok := term.Value.(ast.Var)
		if !ok { // Not a variable -> no substitution
			transformedTerms = append(transformedTerms, term)
			continue
		}

		if sub, ok := processor.localVars[v.String()]; ok {
			transformedTerms = append(transformedTerms, sub)
		} else {
			return nil, errors.Errorf("Undefined variable %s", v.String())
		}
	}
	return transformedTerms, nil
}

func isLocalVarDeclaration(terms []*ast.Term) bool {
	if len(terms) != 3 {
		return false
	}

	// Check if the first Term is "eq" ast.Ref
	if ref, ok := terms[0].Value.(ast.Ref); ok {
		if len(ref) != 1 {
			return false
		}

		if v, ok := ref[0].Value.(ast.Var); !ok || v.String() != "eq" {
			return false
		}
	} else {
		return false
	}

	// Check left side of eq is ast.Var
	if _, ok := terms[1].Value.(ast.Var); !ok {
		return false
	}

	// Check right side of eq is ast.Ref
	if _, ok := terms[2].Value.(ast.Ref); !ok {
		return false
	}

	return true
}
