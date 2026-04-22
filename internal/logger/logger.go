// Package logger provides a thin logging façade used by all packages in this
// module. The default implementation writes to Go's standard log package.
// Call Set to replace it with a platform-specific backend (e.g. OBS's blog()).
package logger

import (
	"fmt"
	"log"
	"sync/atomic"
)

// Logger is the interface that every backend must implement.
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

var current atomic.Pointer[Logger]

func init() {
	l := Logger(&stdLogger{})
	current.Store(&l)
}

// Set replaces the active logger. It must be called before any goroutines that
// log are started, and must not be called with a nil value.
func Set(l Logger) {
	if l == nil {
		panic("logger: Set called with nil Logger")
	}
	current.Store(&l)
}

func active() Logger { return *current.Load() }

func Debug(msg string)               { active().Debug(msg) }
func Debugf(format string, a ...any) { active().Debug(fmt.Sprintf(format, a...)) }
func Info(msg string)                { active().Info(msg) }
func Infof(format string, a ...any)  { active().Info(fmt.Sprintf(format, a...)) }
func Warn(msg string)                { active().Warn(msg) }
func Warnf(format string, a ...any)  { active().Warn(fmt.Sprintf(format, a...)) }
func Error(msg string)               { active().Error(msg) }
func Errorf(format string, a ...any) { active().Error(fmt.Sprintf(format, a...)) }

// stdLogger is the default backend, backed by Go's log package.
// Debug messages are suppressed; set a custom Logger to expose them.
type stdLogger struct{}

func (s *stdLogger) Debug(string)     {}
func (s *stdLogger) Info(msg string)  { log.Println(msg) }
func (s *stdLogger) Warn(msg string)  { log.Println("[WARN]", msg) }
func (s *stdLogger) Error(msg string) { log.Println("[ERROR]", msg) }
