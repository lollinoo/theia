package logging

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel atomic.Int32

func init() {
	currentLevel.Store(int32(LevelInfo))
}

func Configure(raw string) Level {
	level := ParseLevel(raw)
	currentLevel.Store(int32(level))
	if level == LevelDebug {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}
	return level
}

func ParseLevel(raw string) Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "info", "":
		return LevelInfo
	default:
		return LevelInfo
	}
}

func Enabled(level Level) bool {
	return level >= Level(currentLevel.Load())
}

func logf(level Level, prefix string, format string, args ...any) {
	if Enabled(level) {
		_ = log.Output(3, prefix+" "+fmt.Sprintf(format, args...))
	}
}

func Debugf(format string, args ...any) {
	logf(LevelDebug, "DEBUG", format, args...)
}

func Infof(format string, args ...any) {
	logf(LevelInfo, "INFO", format, args...)
}

func Warnf(format string, args ...any) {
	logf(LevelWarn, "WARN", format, args...)
}

func Errorf(format string, args ...any) {
	logf(LevelError, "ERROR", format, args...)
}
