// Package logging provides a small file logger for AWSX. Errors are always
// written; debug entries only when enabled. Credentials and tokens are never
// written — a redactor masks anything that looks like one as a safety net.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Logger writes timestamped entries to an underlying writer.
type Logger struct {
	w      io.Writer
	debug  bool
	closer io.Closer
}

// Dir returns the AWSX config/log directory (XDG_CONFIG_HOME or ~/.config).
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "awsx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".awsx")
	}
	return filepath.Join(home, ".config", "awsx")
}

// Path is the log file location.
func Path() string { return filepath.Join(Dir(), "awsx.log") }

// New opens the log file (appending) and returns a Logger. If the file cannot
// be opened, logging is silently disabled so it never breaks the app.
func New(debug bool) *Logger {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return &Logger{debug: debug}
	}
	f, err := os.OpenFile(Path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return &Logger{debug: debug}
	}
	return &Logger{w: f, debug: debug, closer: f}
}

func newWithWriter(w io.Writer, debug bool) *Logger {
	return &Logger{w: w, debug: debug}
}

// Error writes an error entry (always, when a writer is available).
func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", fmt.Sprintf(format, args...))
}

// Debug writes a debug entry only when debug logging is enabled.
func (l *Logger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.write("DEBUG", fmt.Sprintf(format, args...))
}

func (l *Logger) write(level, msg string) {
	if l == nil || l.w == nil {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s [%s] %s\n", time.Now().Format(time.RFC3339), level, redact(msg))
}

// Close closes the underlying file, if any.
func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

var (
	reKeyValue = regexp.MustCompile(`(?i)(aws_(?:access_key_id|secret_access_key|session_token)|accesstoken|secret|token|password)\s*[=:]\s*\S+`)
	reAccessID = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
)

// redact masks anything resembling a credential or token.
func redact(s string) string {
	s = reKeyValue.ReplaceAllString(s, "$1=[REDACTED]")
	s = reAccessID.ReplaceAllString(s, "[REDACTED]")
	return s
}
