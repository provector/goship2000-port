// Package diagnostics is operational only: no simulation, RNG, record or career ownership.
package diagnostics

import (
	"context"
	"fmt"
	"gunship2000-assets/game/configuration"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

var active atomic.Pointer[Runtime]

type ConsoleFactory func(func(error)) (io.WriteCloser, error)
type Options struct {
	AsyncFile bool
	Console   io.Writer
	Detached  ConsoleFactory
	Emergency io.Writer
}
type Runtime struct {
	fileQueue    chan string
	fileDone     chan struct{}
	mu           sync.Mutex
	logger       *slog.Logger
	level        slog.Level
	file         *rotatingFile
	console      io.Writer
	consoleClose io.Closer
	consoleOK    bool
	consoleEnded bool
	emergency    io.Writer
	failed       map[string]bool
	notices      []string
}

func Enabled(level slog.Level) bool {
	r := active.Load()
	return (r != nil && level >= r.level) || (currentSession.Load() != nil && level >= slog.LevelInfo)
}
func ConsoleActive() bool {
	r := active.Load()
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.consoleOK
}
func Log(level slog.Level, subsystem, message string, args ...any) {
	if currentSession.Load() != nil && level >= slog.LevelInfo {
		recordHistory(string(bound([]byte(fmt.Sprintf("%s %s %s %s %v\n", time.Now().UTC().Format(time.RFC3339Nano), level, subsystem, message, args)))))
	}
	r := active.Load()
	if r == nil || level < r.level {
		return
	}
	all := make([]any, 0, len(args)+2)
	all = append(all, "subsystem", subsystem)
	all = append(all, args...)
	r.logger.Log(context.Background(), level, message, all...)
}
func Info(subsystem, message string, args ...any)  { Log(slog.LevelInfo, subsystem, message, args...) }
func Warn(subsystem, message string, args ...any)  { Log(slog.LevelWarn, subsystem, message, args...) }
func Error(subsystem, message string, args ...any) { Log(slog.LevelError, subsystem, message, args...) }
func Debug(subsystem, message string, args ...any) { Log(slog.LevelDebug, subsystem, message, args...) }

// Existing reports one structured optional record and preserves one essential
// legacy console message when optional Show Log is off/unavailable.
func Existing(level slog.Level, subsystem, message string, err error, w io.Writer) {
	Log(level, subsystem, message, "error", err)
	if !ConsoleActive() && w != nil {
		if err != nil {
			fmt.Fprintf(w, "%s: %v\n", message, err)
		} else {
			fmt.Fprintln(w, message)
		}
	}
}

// ExistingLine preserves an established CLI line whose formatting differs from
// the structured fields. Only one visible copy is emitted with Show Log on.
func ExistingLine(level slog.Level, subsystem, message string, err error, legacy string, w io.Writer) {
	Log(level, subsystem, message, "error", err)
	if !ConsoleActive() && w != nil {
		fmt.Fprintln(w, legacy)
	}
}
func Start(directory string, c configuration.Logging, o Options) *Runtime {
	if !c.ShowLog && !c.WriteLog {
		return nil
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Level))
	r := &Runtime{level: level, emergency: o.Emergency, failed: map[string]bool{}}
	r.logger = slog.New(slog.NewTextHandler(r, &slog.HandlerOptions{Level: level}))
	if c.ShowLog {
		if o.Console != nil {
			r.console = o.Console
			r.consoleOK = true
		} else if o.Detached != nil {
			// Failure callbacks may arrive during construction; all fields are guarded.
			writer, e := o.Detached(func(err error) {
				r.mu.Lock()
				defer r.mu.Unlock()
				r.consoleOK = false
				r.consoleEnded = true
				if err != nil {
					r.failLocked("console", err)
				}
			})
			r.mu.Lock()
			if e != nil {
				r.failLocked("console", e)
			} else {
				r.console = writer
				r.consoleClose = writer
				r.consoleOK = !r.failed["console"] && !r.consoleEnded
			}
			r.mu.Unlock()
		} else {
			r.failLocked("console", fmt.Errorf("no console backend available"))
		}
	}
	if c.WriteLog {
		f, e := openRotating(configuration.Relative(directory, "logs"), c.FileName, int64(c.FileLimitMB)*500000)
		if e != nil {
			r.mu.Lock()
			r.failLocked("file", fmt.Errorf("%s: %w; check permissions or select another Log File Name in SETUP", configuration.LogPath(directory, c), e))
			r.mu.Unlock()
		} else {
			r.mu.Lock()
			r.file = f
			if o.AsyncFile {
				r.startFileWorker()
			}
			r.mu.Unlock()
		}
	}
	active.Store(r)
	return r
}
func bound(b []byte) []byte {
	if len(b) <= MaxRecordBytes {
		return b
	}
	suffix := []byte(" [record truncated]\n")
	n := MaxRecordBytes - len(suffix)
	for n > 0 && !utf8.Valid(b[:n]) {
		n--
	}
	out := make([]byte, 0, MaxRecordBytes)
	out = append(out, b[:n]...)
	return append(out, suffix...)
}
func (r *Runtime) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := bound(b)
	if r.file != nil {
		r.fileRecord(record)
	}
	if r.consoleOK && r.console != nil {
		if _, e := r.console.Write(record); e != nil {
			r.consoleOK = false
			r.failLocked("console", e)
		}
	}
	// Destination faults are isolated; slog must not affect its caller's outcome.
	return len(b), nil
}
func (r *Runtime) failLocked(destination string, e error) {
	if r.failed[destination] {
		return
	}
	r.failed[destination] = true
	status := "Log destination disabled; game continues"
	if destination == "file queue" {
		status = "Text log queue saturated; recent records omitted"
	}
	text := fmt.Sprintf("time=%s level=ERROR subsystem=logging msg=%q destination=%s error=%q\n", time.Now().Format(time.RFC3339Nano), status, destination, e.Error())
	text = string(bound([]byte(text)))
	recordHistory(text)
	Report("log-"+destination, e.Error(), "An optional log output failed. The game can continue.")
	switch {
	case destination != "console" && r.consoleOK && r.console != nil:
		_, _ = io.WriteString(r.console, text)
	case r.emergency != nil:
		_, _ = io.WriteString(r.emergency, text)
	default:
		if len(r.notices) < 2 {
			r.notices = append(r.notices, strings.TrimSpace(text))
		}
	}
	// A console fault remains diagnosable through an independent healthy file.
	if destination == "console" && r.file != nil {
		r.fileRecord([]byte(text))
	}
}
func TakeNotice() string {
	if s := currentSession.Load(); s != nil {
		s.mu.Lock()
		if len(s.notices) > 0 {
			msg := s.notices[0]
			s.notices = s.notices[1:]
			s.mu.Unlock()
			return msg
		}
		s.mu.Unlock()
	}
	r := active.Load()
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.notices) == 0 {
		return ""
	}
	s := r.notices[0]
	r.notices = r.notices[1:]
	return s
}
func (r *Runtime) Close() {
	if r == nil {
		return
	}
	active.CompareAndSwap(r, nil)
	r.mu.Lock()
	if r.fileQueue != nil {
		queue := r.fileQueue
		file := r.file
		r.file = nil
		r.fileQueue = nil
		close(queue)
		r.mu.Unlock()
		<-r.fileDone
		r.mu.Lock()
		r.file = file
	}
	f, c := r.file, r.consoleClose
	r.file = nil
	r.mu.Unlock()
	if f != nil {
		if e := f.Close(); e != nil {
			r.mu.Lock()
			r.failLocked("file", e)
			r.mu.Unlock()
		}
	}
	r.mu.Lock()
	r.consoleOK = false
	r.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// VisibleOutput recognizes a terminal or pipe used by IDEs/shell pipelines.
// Desktop /dev/null and system journal sockets use the detached console instead.
func VisibleOutput(f *os.File) bool {
	i, e := f.Stat()
	if e != nil {
		return false
	}
	if i.Mode()&os.ModeNamedPipe != 0 {
		return true
	}
	return terminalVisible(f)
}
