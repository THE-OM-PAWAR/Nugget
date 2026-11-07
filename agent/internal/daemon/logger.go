package daemon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MaxLogSizeBytes = 10 * 1024 * 1024 // 10 MB
	MaxLogFiles     = 3
)

// Logger writes structured timestamped log messages with automatic file rotation.
type Logger struct {
	LogDir    string
	FilePath  string
	file      *os.File
	out       io.Writer
	mu        sync.Mutex
	bytesSent int64
}

// NewLogger creates a new structured logger writing to <logDir>/nugget.log.
func NewLogger(logDir string, alsoStdout bool) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	filePath := filepath.Join(logDir, "nugget.log")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}

	fi, _ := f.Stat()
	var initialSize int64
	if fi != nil {
		initialSize = fi.Size()
	}

	var writer io.Writer = f
	if alsoStdout {
		writer = io.MultiWriter(f, os.Stdout)
	}

	return &Logger{
		LogDir:    logDir,
		FilePath:  filePath,
		file:      f,
		out:       writer,
		bytesSent: initialSize,
	}, nil
}

// Close flushes and closes the active log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) log(level, format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	msg := fmt.Sprintf(format, args...)
	entry := fmt.Sprintf("[%s] [%-5s] %s\n", timestamp, level, msg)

	n, _ := l.out.Write([]byte(entry))
	l.bytesSent += int64(n)

	if l.bytesSent >= MaxLogSizeBytes {
		l.rotate()
	}
}

// rotate performs log rotation by shifting existing logs.
func (l *Logger) rotate() {
	if l.file != nil {
		_ = l.file.Close()
	}

	for i := MaxLogFiles - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.FilePath, i)
		newPath := fmt.Sprintf("%s.%d", l.FilePath, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	_ = os.Rename(l.FilePath, fmt.Sprintf("%s.1", l.FilePath))

	f, err := os.OpenFile(l.FilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err == nil {
		l.file = f
		l.out = f
		l.bytesSent = 0
	}
}

// Infof writes an INFO level log.
func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", format, args...)
}

// Warnf writes a WARN level log.
func (l *Logger) Warnf(format string, args ...any) {
	l.log("WARN", format, args...)
}

// Errorf writes an ERROR level log.
func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", format, args...)
}
