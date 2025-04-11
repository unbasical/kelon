package logging

import (
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// LabelComponent - Label for component logs
const LabelComponent string = "component"

// LabelPath - Label for decision path
const LabelPath string = "path"

// LabelMethod - Label for decision method
const LabelMethod string = "method"

// LabelDuration - Label for decision duration
const LabelDuration string = "duration"

// LabelDecision - Label for decision result
const LabelDecision string = "decision"

// LabelReason - Label for decision reason
const LabelReason string = "reason"

// LabelError - Label for all errors
const LabelError string = "error"

// LabelCorrelation - Label for multiline error logs
const LabelCorrelation = "correlationId"

// LogAccessDecision formats the decision and logs it
func LogAccessDecision(accessDecisionLogLevel, decision, component string, additionalFields log.Fields) {
	if checkAccessDecisionLogLevel(accessDecisionLogLevel, decision) {
		additionalFields[LabelComponent] = component
		additionalFields[LabelDecision] = decision

		log.WithFields(additionalFields).Info("Access decision:")
	}
}

func checkAccessDecisionLogLevel(logLevel, decision string) bool {
	return logLevel == "ALL" || decision == logLevel
}

// LogWithCorrelationID creates a new log entry which contains the correlation ID
// Useful, if multiple lines should be logged, and it should be clear, that they belong together
func LogWithCorrelationID(correlation uuid.UUID) *log.Entry {
	return log.WithField(LabelCorrelation, correlation.String())
}

// LogForComponent creates a new log entry containing the component label
func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}
