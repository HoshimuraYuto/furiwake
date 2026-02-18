package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
)

type Logger struct {
	mu      sync.Mutex
	logFile *os.File
}

func NewLogger() *Logger {
	l := &Logger{}
	f, err := os.OpenFile("furiwake-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err == nil {
		l.logFile = f
	}
	return l
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(colorGreen, "INFO", format, args...)
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(colorYellow, "WARN", format, args...)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(colorRed, "ERROR", format, args...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(colorCyan, "DEBUG", format, args...)
}

func (l *Logger) log(color, level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	// Console: INFO/WARN/ERROR only (skip DEBUG to reduce noise)
	if level != "DEBUG" {
		fmt.Fprintf(os.Stdout, "%s[%s] %-5s%s %s\n", color, ts, level, colorReset, msg)
	}
	// File: all levels including DEBUG
	if l.logFile != nil {
		fmt.Fprintf(l.logFile, "[%s] %-5s %s\n", ts, level, msg)
	}
}

func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
