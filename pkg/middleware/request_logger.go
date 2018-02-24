package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
)

var static = []string{".css", ".js", ".png", ".jpg", ".jpeg", ".ico"}

// RequestLogger implements chi/middleware.LogFormatter interface for requests logging
type RequestLogger struct{}

// NewRequestLogger creates new logger request middleware instance
func NewRequestLogger() *RequestLogger {
	return &RequestLogger{}
}

// LoggerEntry implements chi/middleware.LogEntry interface for requests logging
type LoggerEntry struct {
	logger *zap.Logger
	path   string
}

// NewLogEntry initiates the beginning of a new LogEntry per request.
func (l *RequestLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &LoggerEntry{path: r.URL.Path}

	entry.logger = GetRequestLogger(r.Context()).With(
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("remote-addr", r.RemoteAddr),
		zap.String("user-agent", r.UserAgent()),
	)

	entry.logger.Debug("Start serving request")

	return entry
}

// Write records the final log when a request completes
func (l *LoggerEntry) Write(status, bytes int, elapsed time.Duration) {
	l.logger = l.logger.With(
		zap.Int("code", status),
		zap.Int("bytes_length", bytes),
		zap.Duration("elapsed_ms", elapsed),
	)

	msg := "Finished serving request"
	for i := range static {
		if strings.HasSuffix(l.path, static[i]) {
			l.logger.Debug(msg)
			return
		}
	}

	l.logger.Info(msg)
}

// Panic records the final log when a request completes
func (l *LoggerEntry) Panic(v interface{}, stack []byte) {
	l.logger.Error("Panic while serving request", zap.ByteString("stack", stack), zap.Any("panic", v))
}
