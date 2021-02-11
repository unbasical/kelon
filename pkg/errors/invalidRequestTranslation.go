package errors

import "fmt"

// Error thrown if query db translation failed
type InvalidRequestTranslation struct {
	Msg   string
	Cause error
}

func (err InvalidRequestTranslation) Error() string {
	if err.Cause != nil {
		return err.Cause.Error()
	}
	return fmt.Sprintf("PolicyCompiler: Error during ast translation: %s", err.Msg)
}
