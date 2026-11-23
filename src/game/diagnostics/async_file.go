package diagnostics

import "fmt"

// Only the writer worker owns the file during normal gameplay. It batches
// queued records; a saturated optional sink cannot stall the input thread.
func (r *Runtime) startFileWorker() {
	r.fileQueue = make(chan string, 256)
	r.fileDone = make(chan struct{})
	queue, file := r.fileQueue, r.file
	go func() {
		defer close(r.fileDone)
		healthy := true
		for line := range queue {
			if !healthy {
				continue
			}
			if _, e := file.Write([]byte(line)); e != nil {
				healthy = false
				r.mu.Lock()
				r.failLocked("file", e)
				r.mu.Unlock()
			}
		}
	}()
}
func (r *Runtime) fileRecord(line []byte) {
	if r.fileQueue != nil {
		select {
		case r.fileQueue <- string(line):
		default:
			if !r.failed["file queue"] {
				r.failLocked("file queue", fmt.Errorf("text log queue full; records omitted"))
			}
		}
	} else if _, e := r.file.Write(line); e != nil {
		r.file.Close()
		r.file = nil
		r.failLocked("file", e)
	}
}
