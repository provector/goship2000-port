// Package presentationstats measures completed window presentations and total
// process CPU. One second of CPU per second of elapsed time is 100%.
package presentationstats

import (
	"fmt"
	"time"
)

type Meter struct {
	start  time.Time
	cpu    time.Duration
	valid  bool
	frames int
	Text   string
}

func Start(enabled bool) *Meter {
	if !enabled {
		return nil
	}
	m := &Meter{Text: "FPS -- | CPU N/A"}
	m.start = time.Now()
	m.cpu, m.valid = processCPU()
	return m
}

func (m *Meter) Presented() {
	if m != nil {
		m.sample(time.Now(), processCPU)
	}
}

func (m *Meter) sample(now time.Time, read func() (time.Duration, bool)) {
	m.frames++
	elapsed := now.Sub(m.start)
	if elapsed < time.Second {
		return
	}
	cpu, ok := read()
	fps := float64(m.frames) / elapsed.Seconds()
	if ok && m.valid && cpu >= m.cpu {
		m.Text = fmt.Sprintf("FPS %.1f | CPU %.0f%%", fps, 100*float64(cpu-m.cpu)/float64(elapsed))
	} else {
		m.Text = fmt.Sprintf("FPS %.1f | CPU N/A", fps)
	}
	m.start, m.cpu, m.valid, m.frames = now, cpu, ok, 0
}
