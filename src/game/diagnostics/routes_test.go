package diagnostics

import (
	"bytes"
	"errors"
	"gunship2000-assets/game/configuration"
	"io"
	"os"
	"testing"
)

type routesSink struct{ bytes.Buffer }

func (*routesSink) Close() error { return nil }
func TestTargetRoutingAndIndependentFile(t *testing.T) {
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	defer w.Close()
	for _, target := range []string{"system", "custom"} {
		for _, show := range []bool{false, true} {
			calls := 0
			sink := &routesSink{}
			c := configuration.DefaultLogging()
			c.Level = "INFO"
			c.ShowLog = show
			c.WriteLog = true
			c.TerminalTarget = target
			dir := t.TempDir()
			rt := Start(dir, c, Destinations(c, w, w, func(func(error)) (io.WriteCloser, error) { calls++; return sink, nil }))
			Info("q14", "one optional record")
			rt.Close()
			expected := 0
			if show && target == "custom" {
				expected = 1
			}
			if calls != expected {
				t.Fatal(target, show, calls)
			}
			data, e := os.ReadFile(configuration.LogPath(dir, c))
			if e != nil || !bytes.Contains(data, []byte("one optional record")) {
				t.Fatal(e, string(data))
			}
			if show && target == "custom" && bytes.Count(sink.Bytes(), []byte("one optional record")) != 1 {
				t.Fatal(sink.String())
			}
		}
	}
}

func TestFailedLogWindowFallsBackToTerminal(t *testing.T) {
	for _, when := range []string{"open", "early exit", "late exit", "user closed"} {
		t.Run(when, func(t *testing.T) {
			var terminal bytes.Buffer
			var exited func(error)
			failed := errors.New("window creation failed")
			c := configuration.DefaultLogging()
			c.ShowLog, c.Level = true, "INFO"
			r := Start(t.TempDir(), c, Options{ConsoleFallback: &terminal, Detached: func(exit func(error)) (io.WriteCloser, error) {
				exited = exit
				if when == "open" {
					return nil, failed
				}
				if when == "early exit" {
					exit(failed)
				}
				return &routesSink{}, nil
			}})
			defer r.Close()
			if when == "late exit" {
				exited(failed)
			}
			if when == "user closed" {
				exited(nil)
			}
			Info("fixture", "subsequent record")
			if got := bytes.Contains(terminal.Bytes(), []byte("subsequent record")); got != (when != "user closed") {
				t.Fatalf("unexpected fallback output %q", terminal.String())
			}
		})
	}
}
