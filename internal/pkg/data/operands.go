package data

import (
	"fmt"
	log "github.com/sirupsen/logrus"
)

type CallMapper interface {
	Handles() string
	Map(args ...string) string
}

type GenericCallHandler struct {
	operator  string
	argsCount int
	handler   func(args ...string) (string, error)
}

func (h GenericCallHandler) Handles() string {
	return h.operator
}

func (h GenericCallHandler) Map(args ...string) string {
	argsLen := len(args)
	if argsLen < h.argsCount || argsLen > (h.argsCount+1) {
		log.Fatalf("Call-handler [%s] had wrong amount of arguments! Expected %d or %d arguments, but got %+v as input.\n", h.operator, h.argsCount, h.argsCount+1, args)
	}

	var (
		result string
		err    error
	)
	// Handle call with default comparison
	if argsLen > h.argsCount {
		result, err = h.handler(args[:argsLen-1]...)
		result = fmt.Sprintf("%s = %s", result, args[argsLen-1])
	} else { // Handle any other
		result, err = h.handler(args...)
	}

	if err != nil {
		log.Fatalf("Call-handler [%s] failed due to error: %s", err.Error())
	}

	return result
}

var MySqlCallHandlers = []CallMapper{
	GenericCallHandler{
		operator:  "abs",
		argsCount: 1,
		handler:   func(args ...string) (string, error) { return fmt.Sprintf("ABS(%s)", args[0]), nil },
	},
	GenericCallHandler{
		operator:  "mul",
		argsCount: 2,
		handler:   func(args ...string) (string, error) { return fmt.Sprintf("%s * %s", args[0], args[1]), nil },
	},
	GenericCallHandler{
		operator:  "div",
		argsCount: 2,
		handler:   func(args ...string) (string, error) { return fmt.Sprintf("%s / %s", args[0], args[1]), nil },
	},
}
