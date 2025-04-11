package util

import log "github.com/sirupsen/logrus"

type UTCFormatter struct {
	log.Formatter
}

// Format formats the time on log entries for the UTC location
func (u UTCFormatter) Format(e *log.Entry) ([]byte, error) {
	e.Time = e.Time.UTC()
	return u.Formatter.Format(e)
}
