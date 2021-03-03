package logging

import (
	log "github.com/sirupsen/logrus"
)

// Label for component logs
const LabelComponent string = "component"

// Label for decision path
const LabelPath string = "path"

// Label for decision method
const LabelMethod string = "method"

// Label for decision duration
const LabelDuration string = "duration"

// Label for decision decision
const LabelDecision string = "decision"

func LogAccessDecision(accessDecissionLogLevel, path, method, duration, decision, component string) {
	if checkAccessDecisionLogLevel(accessDecissionLogLevel, decision) {
		log.WithFields(log.Fields{
			LabelPath:      path,
			LabelMethod:    method,
			LabelDuration:  duration,
			LabelDecision:  decision,
			LabelComponent: component,
		}).Info("Access decision:")
	}
}

func checkAccessDecisionLogLevel(logLevel, decision string) bool {
	return logLevel == "ALL" || decision == logLevel
}

func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}
