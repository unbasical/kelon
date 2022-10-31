package logging

import (
	"github.com/google/uuid"
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

// Label for translation error
const LabelError string = "error"

// Label for multiline error logs
const LabelCorrelation = "correlationId"

func LogAccessDecision(accessDecisionLogLevel, path, method, duration, decision, component string) {
	if checkAccessDecisionLogLevel(accessDecisionLogLevel, decision) {
		log.WithFields(log.Fields{
			LabelPath:      path,
			LabelMethod:    method,
			LabelDuration:  duration,
			LabelDecision:  decision,
			LabelComponent: component,
		}).Info("Access decision:")
	}
}

func LogAccessDecisionError(accessDecisionLogLevel, path, method, duration, err, correlation, decision, component string) {
	if checkAccessDecisionLogLevel(accessDecisionLogLevel, decision) {
		log.WithFields(log.Fields{
			LabelPath:        path,
			LabelMethod:      method,
			LabelDuration:    duration,
			LabelDecision:    decision,
			LabelError:       err,
			LabelCorrelation: correlation,
			LabelComponent:   component,
		}).Warn("Access decision:")
	}
}

func checkAccessDecisionLogLevel(logLevel, decision string) bool {
	return logLevel == "ALL" || decision == logLevel
}

func LogWithCorrelationId(correlation uuid.UUID) *log.Entry {
	return log.WithField(LabelCorrelation, correlation.String())
}

func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}
