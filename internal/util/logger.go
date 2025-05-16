package util

import (
	"fmt"
	"time"
)

// ANSI color codes
const (
	Reset     = "\033[0m"
	Red       = "\033[31m"
	Green     = "\033[32m"
	Yellow    = "\033[33m"
	Blue      = "\033[34m"
	Purple    = "\033[35m"
	Cyan      = "\033[36m"
	Bold      = "\033[1m"
	Underline = "\033[4m"
)

// Log levels
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

type Logger struct {
	verbose bool
	prefix  string
}

// NewLogger creates a new logger instance
func NewLogger(verbose bool, prefix string) *Logger {
	return &Logger{
		verbose: verbose,
		prefix:  prefix,
	}
}

func (l *Logger) log(level int, color string, format string, args ...interface{}) {
	// Skip debug messages if not in verbose mode
	if level == LevelDebug && !l.verbose {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	levelStr := ""

	switch level {
	case LevelDebug:
		levelStr = "DEBUG"
	case LevelInfo:
		levelStr = "INFO"
	case LevelWarn:
		levelStr = "WARN"
	case LevelError:
		levelStr = "ERROR"
	case LevelFatal:
		levelStr = "FATAL"
	}

	// Format the message
	message := fmt.Sprintf(format, args...)

	// Print the formatted log line
	fmt.Printf("%s%s [%s] %s%s: %s%s\n",
		color,
		timestamp,
		levelStr,
		Bold,
		l.prefix,
		Reset+color,
		message+Reset,
	)
}

// Debug logs debug information (only shown when verbose is true)
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, Cyan, format, args...)
}

// Info logs general information
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, Green, format, args...)
}

// Warn logs warnings
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, Yellow, format, args...)
}

// Error logs error messages
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, Red, format, args...)
}

// Fatal logs critical errors
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelFatal, Purple+Bold, format, args...)
}

// Success logs success messages with a green checkmark
func (l *Logger) Success(format string, args ...interface{}) {
	l.log(LevelInfo, Green+Bold, "✓ "+format, args...)
}

// Progress creates a simple progress message
func (l *Logger) Progress(format string, args ...interface{}) {
	l.log(LevelInfo, Cyan, "⟳ "+format, args...)
}
