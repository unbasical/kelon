package logging

import (
	log "github.com/sirupsen/logrus"
)

// Label for component logs
const LabelComponent string = "component"

// Label for request UID
const LabelUID string = "UID"

func LogForComponent(component string) *log.Entry {
	return log.WithField(LabelComponent, component)
}

func LogForComponentAndUID(component string, uid string) *log.Entry {
	return log.WithFields(log.Fields{
		LabelComponent: component,
		LabelUID:       uid,
	})
}
