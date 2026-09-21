//go:build goinject

//inject:github.com/apache/skywalking-go/toolkit/logging
package toolkitactivation

import (
	"github.com/apache/skywalking-go/plugins/core/operator"
)

func Debug(msg string, keyValues ...string) {
	swSendLogEntry(swDebugLevel, msg, keyValues)
}

func Info(msg string, keyValues ...string) {
	swSendLogEntry(swInfoLevel, msg, keyValues)
}

func Warn(msg string, keyValues ...string) {
	swSendLogEntry(swWarnLevel, msg, keyValues)
}

func Error(msg string, keyValues ...string) {
	swSendLogEntry(swErrorLevel, msg, keyValues)
}

//inject:add
const (
	swDebugLevel = "debug"
	swInfoLevel  = "info"
	swWarnLevel  = "warn"
	swErrorLevel = "error"
)

//inject:add
func swSendLogEntry(level string, args ...interface{}) {
	if len(args) == 0 {
		return
	}
	logReporter, ok := operator.GetOperator().LogReporter().(operator.LogReporter)
	if !ok || logReporter == nil {
		return
	}

	msg := args[0].(string)
	labels := swParseLabels(args[1])
	logReporter.ReportLog(logReporter.GetLogContext(true), args[1], level, msg, labels)
}

// swParseLabels parses multiple args into a map of labels
//
//inject:add
func swParseLabels(args interface{}) map[string]string {
	keyValues, ok := args.([]string)
	if !ok || len(keyValues) < 2 {
		return nil
	}

	ret := make(map[string]string)
	for i := 0; i < len(keyValues); i += 2 {
		v1 := keyValues[i]
		v2 := keyValues[i+1]
		ret[v1] = v2
	}

	return ret
}
