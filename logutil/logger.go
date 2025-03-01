package logutil

import (
	"fmt"
	"log"
	"os"
	"runtime"
)

// debugLog writes log messages to /log/debug.log and includes file/line number.
// Public identifiers start with a capital Letter! Hence, DebugLog
func DebugLog(message string, args ...interface{}) {
	f, err := os.OpenFile("/log/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // Fail silently if logging is unavailable
	}
	defer f.Close()

	log.SetOutput(f)

	// Get file and line number
	_, file, line, ok := runtime.Caller(1)
	if ok {
		message = "[" + file + ":" + fmt.Sprintf("%d", line) + "] " + message
	}

	log.Printf(message, args...)
}
