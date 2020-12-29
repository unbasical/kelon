package logging

import (
	log "github.com/sirupsen/logrus"
)

// Label for component logs
const LabelComponent string = "component"

// Label for request UID
const LabelUID string = "UID"

// Label for decision path
const LabelPath string = "Path"

// Label for decision method
const LabelMethod string = "Method"

// Label for decision duration
const LabelDuration string = "Duration"

// Label for decision decision
const LabelDecision string = "Decision"

func LogAccessDecision(accessDecissionLogLevel string, path string, method string, duration string, decision string) *log.Entry {
	if checkAccessDecisionLogLevel(accessDecissionLogLevel, decision) {
		return log.WithFields(log.Fields{
			LabelPath:     path,
			LabelMethod:   method,
			LabelDuration: duration,
			LabelDecision: decision,
		})
	}
	return nil
}

func checkAccessDecisionLogLevel(logLevel string, decision string) bool {
	loggingStatus := false
	switch logLevel {
	case "ALL":
		loggingStatus = true
	default:
		if logLevel == decision {
			loggingStatus = true
		}
	}
	return loggingStatus
}

func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}

func LogForComponentAndUID(component, uid string) *log.Entry {
	return log.WithFields(log.Fields{
		LabelComponent: component,
		LabelUID:       uid,
	})
}
