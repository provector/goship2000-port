//go:build linux

package main

import (
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

type terminalSpec struct {
	name   string
	prefix []string
}

func terminalCommand(exe string, args []string, dir, ready string, lookup func(string) (string, error)) ([]string, error) {
	child := append(append([]string{}, args...), "-terminal-install", dir, "-terminal-child", "-terminal-ready", ready)
	specs := []terminalSpec{{"xdg-terminal-exec", []string{"--dir=" + dir, "--"}}, {"x-terminal-emulator", []string{"-e"}}, {"gnome-terminal", []string{"--working-directory=" + dir, "--"}}, {"konsole", []string{"--icon", "org.goship2000.Setup", "--workdir", dir, "-e"}}, {"xfce4-terminal", []string{"--icon=org.goship2000.Setup", "--disable-server", "--default-working-directory=" + dir, "-x"}}, {"xterm", []string{"-e"}}}
	for _, s := range specs {
		if path, e := lookup(s.name); e == nil {
			return append(append([]string{path}, s.prefix...), append([]string{exe}, child...)...), nil
		}
	}
	return nil, fmt.Errorf("no supported terminal emulator found (xdg-terminal-exec, x-terminal-emulator, gnome-terminal, konsole, xfce4-terminal or xterm)")
}
func signalTerminalReady(path string) error {
	if path == "" {
		return nil
	}
	parent := filepath.Dir(path)
	resolved, e := filepath.EvalSymlinks(parent)
	if e != nil || resolved != parent || filepath.Dir(parent) != os.TempDir() || !strings.HasPrefix(filepath.Base(parent), "goship-setup-terminal-") || filepath.Base(path) != "ready" {
		return fmt.Errorf("invalid private terminal readiness path")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(f, "pid=%d\ntty=%s\n", os.Getpid(), os.Stdin.Name())
	if e != nil {
		f.Close()
		return e
	}
	return f.Close()
}

// Returns handled when the parent handed off, or a failure has been presented.
func ensureTerminal(dir string, args []string, child bool, ready string) (handled bool, code int) {
	if interactiveTerminal() {
		if e := signalTerminalReady(ready); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return true, 1
		}
		return false, 0
	}
	exe, e := os.Executable()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return true, 1
	}
	fail := func(e error) (bool, int) {
		msg := fmt.Sprintf("GoShip 2000 SETUP could not open an interactive terminal.\n\n%s\n\nOpen a terminal and run:\n%q\n\nYour existing configuration and assets were preserved.", e, exe)
		fmt.Fprintln(os.Stderr, msg)
		launchWarning(dir, msg)
		return true, 1
	}
	if child {
		return fail(fmt.Errorf("terminal relaunch did not provide a TTY; automatic relaunch will not repeat"))
	}
	temp, e := os.MkdirTemp("", "goship-setup-terminal-")
	if e != nil {
		return fail(e)
	}
	defer os.RemoveAll(temp) // Only this newly-created private handshake directory.
	marker := filepath.Join(temp, "ready")
	argv, e := terminalCommand(exe, args, dir, marker, exec.LookPath)
	if e != nil {
		return fail(e)
	}
	var stderr terminalErrors
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	// Only the spawned terminal receives the package-local icon/desktop lookup.
	share := filepath.Join(filepath.Dir(exe), "share")
	if _, err := os.Stat(filepath.Join(share, "applications", "org.goship2000.Setup.desktop")); err == nil {
		dirs := os.Getenv("XDG_DATA_DIRS")
		if dirs == "" {
			dirs = "/usr/local/share:/usr/share"
		}
		cmd.Env = append(os.Environ(), "XDG_DATA_DIRS="+share+":"+dirs)
	}
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return fail(fmt.Errorf("start %s: %w", argv[0], e))
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if _, e = os.Stat(marker); e == nil {
				b, _ := os.ReadFile(marker)
				fmt.Fprintf(os.Stderr, "SETUP terminal ready: app=%q %s", dir, b)
				return true, 0
			}
		case e = <-done:
			if e != nil {
				return fail(fmt.Errorf("%s: %w; %s", argv[0], e, stderr.String()))
			}
			// DBus terminal clients can exit before their new TTY child starts. Wait
			// for the child's handshake; this also keeps a go-run executable alive.
		case <-timeout.C:
			return fail(fmt.Errorf("terminal child did not become ready within15seconds; %s", stderr.String()))
		}
	}
}
func launchWarning(dir, message string) {
	for _, v := range []struct {
		name string
		args []string
	}{{"zenity", []string{"--error", "--title=GoShip 2000 Setup", "--text=" + message}}, {"kdialog", []string{"--error", message, "--title", "GoShip 2000 Setup"}}, {"xmessage", []string{"-center", message}}} {
		if p, e := exec.LookPath(v.name); e == nil {
			_ = exec.Command(p, v.args...).Run()
			return
		}
	}
	// Packaged pairs always have this asset-free fallback; no cfg or save owner.
	game := filepath.Join(dir, "gs2000")
	if s, e := os.Stat(game); e == nil && s.Mode().IsRegular() {
		_ = exec.Command(game, "__setup-warning", message).Run()
	}
}

// Terminal clients can keep writing after their readiness handshake. Bound and
// synchronize failure output; it is diagnostic text, never a configuration.
type terminalErrors struct {
	mu   sync.Mutex
	text []byte
}

func (b *terminalErrors) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.text = append(b.text, p...)
	if len(b.text) > 8192 {
		b.text = b.text[len(b.text)-8192:]
	}
	return n, nil
}
func (b *terminalErrors) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.text) }
