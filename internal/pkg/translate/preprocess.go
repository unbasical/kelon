package translate

import (
	"fmt"
	"github.com/open-policy-agent/opa/ast"
	"github.com/pkg/errors"
)

type AstPreprocessor struct {
	tableNames []map[string]string
	tableVars  map[string][]*ast.Term
}

func (processor *AstPreprocessor) Process(queries []ast.Body, datastore string) ([]ast.Body, error) {
	var result []ast.Body
	for _, q := range queries {
		processor.tableNames = append(processor.tableNames, make(map[string]string))
		processor.tableVars = make(map[string][]*ast.Term)
		if trans, err := processor.transform(q, datastore); err == nil {
			if body, ok := trans.(ast.Body); ok {
				result = append(result, body)
			} else {
				return nil, errors.New("Preprocessor: Processing went not es expected! Wrong type was returned after AST-transformation. ")
			}
		} else {
			return nil, errors.Wrap(err, "Preprocessor: ")
		}
	}

	return result, nil
}

func (processor *AstPreprocessor) transform(query interface{}, datastore string) (interface{}, error) {
	expectedDatastore := fmt.Sprintf("\"%s\"", datastore)

	trans := func(node ast.Ref) (ast.Value, error) {
		// Skip operands
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
		if head == "data" && node[1].String() != expectedDatastore {
			return nil, errors.Errorf("Invalid reference: expected [data.%s.<table>] but found reference [%s] \n", datastore, node.String())
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
		// query.
		last := processor.tableNames[len(processor.tableNames)-1]
		if _, ok := last[tableName]; ok {
			return nil, errors.New("invalid reference: self-links not supported")
		} else {
			processor.tableNames[len(processor.tableNames)-1][tableName] = rowId.String()
		}

		// Rewrite ref to remove iterator var. E.g., "data.<datastore>.foo[x].bar" =>
		// "data.foo.bar".
		return ast.Ref{}.Concat(append(prefix, node[4:]...)), nil
	}

	return ast.TransformRefs(query, trans)
}
