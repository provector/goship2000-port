package logview

import (
	"os"
	"testing"
	"time"
)

func init() {
	// A private test-child deliberately never reads its pipe. No window,
	// configuration, data or shell command is involved.
	if os.Getenv("GOSH_Q13_LOG_TEST_CHILD") == "hang" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

func TestStalledConsoleShutdownIsBounded(t *testing.T) {
	t.Setenv("GOSH_Q13_LOG_TEST_CHILD", "hang")
	w, err := Open(func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 128; i++ {
		_, _ = w.Write(make([]byte, 4096))
	}
	start := time.Now()
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatal("stalled owned child blocked shutdown", elapsed)
	}
}
