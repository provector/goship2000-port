package sounds

import (
	"testing"
	"time"

	assetdata "gunship2000-assets/game/assets"
)

func TestActualRotorWorkerPauseAndCleanup(t *testing.T) {
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	r, err := newRotorStream(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	deadline := time.Now().Add(3 * time.Second)
	wait := func() {
		t.Helper()
		for len(r.pcm) == 0 {
			if time.Now().After(deadline) {
				t.Fatal("bounded audio worker timed out", r.Status())
			}
			if r.Status().Failure != "" {
				t.Fatal(r.Status())
			}
			time.Sleep(time.Millisecond)
		}
	}
	p := make([]float32, rotorBlockFrames)
	nonzero := false
	for i := 0; i < 16; i++ {
		wait()
		r.Read(p)
		for _, v := range p {
			nonzero = nonzero || v != 0
		}
	}
	if !nonzero {
		t.Fatal("actual retained worker produced no source sound")
	}
	r.SetPaused(true)
	position := r.position
	for i := 0; i < 3; i++ {
		r.Read(p)
		for _, v := range p {
			if v != 0 {
				t.Fatal("paused sound leaked")
			}
		}
	}
	if r.position != position {
		t.Fatal("pause consumed queued PCM")
	}
	if err = r.Command(15); err != nil {
		t.Fatal(err)
	}
	r.SetPaused(false)
	wait()
	r.Read(p)
	r.Close()
	r.Close()
	select {
	case <-r.done:
	default:
		t.Fatal("owned worker did not close")
	}
	if err = r.Command(0); err == nil {
		t.Fatal("closed worker accepted command")
	}
}

// A FARP stop can run the driver fully down. A single start command must
// bring back sustained PCM on the same stream without further key presses.
func TestRotorRestartsAfterSilentTail(t *testing.T) {
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	r, e := newRotorStream(source[l.DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	block := func() float32 {
		t.Helper()
		select {
		case b := <-r.pcm:
			var peak float32
			for _, v := range b {
				peak = max(peak, v, -v)
			}
			return peak
		case <-r.done:
			t.Fatal(r.Status())
		case <-time.After(3 * time.Second):
			t.Fatal("no PCM")
		}
		return 0
	}
	for i := 0; i < 32; i++ {
		block()
	}
	if e = r.Command(0); e != nil {
		t.Fatal(e)
	}
	quiet := 0
	for i := 0; i < 3000 && quiet < 12; i++ {
		if block() < .0001 {
			quiet++
		} else {
			quiet = 0
		}
	}
	if quiet < 12 {
		t.Fatal("no shutdown", r.Status())
	}
	if e = r.Command(1); e != nil {
		t.Fatal(e)
	}
	loud := 0
	for i := 0; i < 160; i++ {
		if block() > .001 {
			loud++
		}
	}
	if loud < 20 || r.Status().Commands != 3 {
		t.Fatal(loud, r.Status())
	}
}
