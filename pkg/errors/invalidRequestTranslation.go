package errors

import (
	"fmt"
)

// Error thrown if query db translation failed
type InvalidRequestTranslation struct {
	Msg    string
	Causes []string
}

func (err InvalidRequestTranslation) Error() string {
	if len(err.Causes) != 0 {
		return fmt.Sprintf("AstTranslator: Found %d errors during processing policies", len(err.Causes))
	}

	return fmt.Sprintf("PolicyCompiler: Error during ast translation: %s", err.Msg)
}
