package diagnostics

import (
	"bytes"
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
