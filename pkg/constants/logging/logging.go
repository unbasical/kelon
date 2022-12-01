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

// Label for decision reason
const LabelReason string = "reason"

// Label for translation error
const LabelError string = "error"

// Label for multiline error logs
const LabelCorrelation = "correlationId"

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

func LogWithCorrelationID(correlation uuid.UUID) *log.Entry {
	return log.WithField(LabelCorrelation, correlation.String())
}

func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}
