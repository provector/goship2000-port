//go:build linux

package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestTerminalArgvAndMissingFallback(t *testing.T) {
	original := []string{"-app-dir", "/owned/A B", "-terminal-ready", "old"}
	argv, e := terminalCommand("/owned/Game With Spaces/setup", original, "/owned/Runtime With Spaces", "/tmp/goship-setup-terminal-x/ready", func(s string) (string, error) {
		if s == "xterm" {
			return "/usr/bin/xterm", nil
		}
		return "", errors.New("not installed")
	})
	want := append([]string{"/usr/bin/xterm", "-e", "/owned/Game With Spaces/setup"}, original...)
	want = append(want, "-terminal-install", "/owned/Runtime With Spaces", "-terminal-child", "-terminal-ready", "/tmp/goship-setup-terminal-x/ready")
	if e != nil || !reflect.DeepEqual(argv, want) {
		t.Fatal(argv, e)
	}
	if _, e = terminalCommand("x", nil, "/owned", "", func(string) (string, error) { return "", errors.New("missing") }); e == nil {
		t.Fatal("no fallback diagnosis")
	}
	if signalTerminalReady("/etc/ready") == nil {
		t.Fatal("unsafe handshake")
	}
}
