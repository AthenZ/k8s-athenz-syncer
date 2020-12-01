/*
Copyright 2019, Oath Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package log

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mash/go-accesslog"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var log *logrus.Logger

func newLogger(logFile, level string) *logrus.Logger {
	logger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    1, // Mb
		MaxBackups: 5,
		MaxAge:     28, // Days
	}

	fileWriter := io.MultiWriter(os.Stdout, logger)

	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Warnln("Could not parse log level, defaulting to info. Error:", err.Error())
		logLevel = logrus.InfoLevel
	}

	formatter := &logrus.TextFormatter{
		ForceColors:            true,
		DisableSorting:         true,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	}

	l := &logrus.Logger{
		Out:       fileWriter,
		Formatter: formatter,
		Level:     logLevel,
	}
	l.SetNoLock()

	dir := filepath.Dir(logFile)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		logrus.Errorln("Could not mkdir for log file, defaulting to stdout logging. Error:", err.Error())
		l.Out = os.Stdout
	}
	return l
}

// InitLogger initializes a logger object with log rotation
func InitLogger(logFile, level string) {
	log = newLogger(logFile, level)
}

type AccessLogger struct {
	access *logrus.Logger
}

func (l *AccessLogger) Log(record accesslog.LogRecord) {
	l.access.Printf("%s %s %d %v %v", record.Method, record.Uri, record.Status, record.ElapsedTime, record.CustomRecords)
}

// InitAccessLogger returns a handler that wraps the supplied delegate with access logging.
func InitAccessLogger(h http.Handler, logFile, level string) http.Handler {
	l := &AccessLogger{
		access: newLogger(logFile, level),
	}
	return accesslog.NewLoggingHandler(h, l)
}

// Debugf - Debugf function
func Debugf(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// Infof - Infof function
func Infof(format string, args ...interface{}) {
	log.Infof(format, args...)
}

// Printf - Printf function
func Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// Warnf - Warnf function
func Warnf(format string, args ...interface{}) {
	log.Warnf(format, args...)
}

// Warningf - Warningf function
func Warningf(format string, args ...interface{}) {
	log.Warningf(format, args...)
}

// Errorf - Errorf function
func Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

// Fatalf - Fatalf function
func Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

// Panicf - Panicf function
func Panicf(format string, args ...interface{}) {
	log.Panicf(format, args...)
}

// Debug - Debug function
func Debug(args ...interface{}) {
	log.Debug(args...)
}

// Info - Info function
func Info(args ...interface{}) {
	log.Info(args...)
}

// Print - Print function
func Print(args ...interface{}) {
	log.Print(args...)
}

// Warn - Warn function
func Warn(args ...interface{}) {
	log.Warn(args...)
}

// Warning - Warning function
func Warning(args ...interface{}) {
	log.Warning(args...)
}

// Error - Error function
func Error(args ...interface{}) {
	log.Error(args...)
}

// Fatal - Fatal function
func Fatal(args ...interface{}) {
	log.Fatal(args...)
}

// Panic - Panic function
func Panic(args ...interface{}) {
	log.Panic(args...)
}

// Debugln - Debugln function
func Debugln(args ...interface{}) {
	log.Debugln(args...)
}

// Infoln - Infoln function
func Infoln(args ...interface{}) {
	log.Infoln(args...)
}

// Println - Println function
func Println(args ...interface{}) {
	log.Println(args...)
}

// Warnln - Warnln function
func Warnln(args ...interface{}) {
	log.Warnln(args...)
}

// Warningln - Warningln function
func Warningln(args ...interface{}) {
	log.Warningln(args...)
}

// Errorln - Errorln function
func Errorln(args ...interface{}) {
	log.Errorln(args...)
}

// Fatalln - Fatalln function
func Fatalln(args ...interface{}) {
	log.Fatalln(args...)
}

// Panicln - Panicln function
func Panicln(args ...interface{}) {
	log.Panicln(args...)
}
