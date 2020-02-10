package errors

import "fmt"

// Error thrown if any input is invalid
type InvalidInput struct {
	Msg   string
	Cause error
}

func (err InvalidInput) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("Invalid input [%s] due to error: %s", err.Msg, err.Cause.Error())
	}
	return fmt.Sprintf("Invalid input [%s]", err.Msg)
}
