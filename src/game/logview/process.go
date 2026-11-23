package logview

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// A bounded queue keeps a slow/closed log window out of the game update path.
type pipeConsole struct {
	pipe   io.WriteCloser
	queue  chan []byte
	stop   chan struct{}
	done   chan struct{}
	exited chan struct{}
	once   sync.Once
	closed atomic.Bool
	cmd    *exec.Cmd
	err    error
}

func Open(onError func(error)) (io.WriteCloser, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(exe, "__log-console")
	p, e := cmd.StdinPipe()
	if e != nil {
		return nil, e
	}
	var stderr tailBuffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		p.Close()
		return nil, e
	}
	c := &pipeConsole{pipe: p, queue: make(chan []byte, 128), stop: make(chan struct{}), done: make(chan struct{}), exited: make(chan struct{}), cmd: cmd}
	go func() {
		c.err = cmd.Wait()
		c.closed.Store(true)
		p.Close()
		close(c.exited)
		if c.err != nil {
			onError(fmt.Errorf("log window exited: %w; %s", c.err, stderr.String()))
		} else {
			onError(nil)
		}
	}()
	go func() {
		defer close(c.done)
		for {
			select {
			case <-c.stop:
				for {
					select {
					case b := <-c.queue:
						if _, e := c.pipe.Write(b); e != nil {
							return
						}
					default:
						return
					}
				}
			case b := <-c.queue:
				if _, e := c.pipe.Write(b); e != nil {
					// Wait owns the exit notification. Do not block this writer:
					// Close must still reach its bounded child shutdown timeout.
					return
				}
			}
		}
	}()
	return c, nil
}
func (c *pipeConsole) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return len(b), nil
	}
	copy := append([]byte(nil), b...)
	select {
	case c.queue <- copy:
		return len(b), nil
	default:
		return 0, errors.New("log console queue full; file/game continue; raise Log Level or reopen Show Log")
	}
}
func (c *pipeConsole) Close() error {
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.stop)
		select {
		case <-c.done:
		case <-time.After(time.Second):
			c.pipe.Close()
			<-c.done
		}
		c.pipe.Close()
		select {
		case <-c.exited:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
			<-c.exited
		}

	})
	return nil
}

type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(b)
	if n > 4096 {
		b = b[n-4096:]
	}
	t.b = append(t.b, b...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return n, nil
}
func (t *tailBuffer) String() string { t.mu.Lock(); defer t.mu.Unlock(); return string(t.b) }
