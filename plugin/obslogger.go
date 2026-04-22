package main

import "github.com/icedream/spotify-lyrics-widget/internal/logger"

// obsLogger routes all log output through OBS's blog() infrastructure.
// Level values are initialised from the C LOG_* constants in main.go.
type obsLogger struct{}

func (o *obsLogger) Debug(msg string) { blog(logLevelDebug, msg) }
func (o *obsLogger) Info(msg string)  { blog(logLevelInfo, msg) }
func (o *obsLogger) Warn(msg string)  { blog(logLevelWarn, msg) }
func (o *obsLogger) Error(msg string) { blog(logLevelError, msg) }

// compile-time check
var _ logger.Logger = (*obsLogger)(nil)
