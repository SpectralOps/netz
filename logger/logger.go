package log

import (
	"os"

	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var Logger *logrus.Logger

func SetLogger(debug bool) {
	level := logrus.InfoLevel
	if debug {
		level = logrus.DebugLevel
	}

	Logger = &logrus.Logger{
		Out:   os.Stderr,
		Level: level,
		Formatter: &prefixed.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			FullTimestamp:   true,
			ForceFormatting: true,
		},
	}
}
