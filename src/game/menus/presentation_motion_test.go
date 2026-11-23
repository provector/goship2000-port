package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// Measures actual indexed world-frame changes, not a target-FPS counter. A frame
// with no consumed operation may interpolate in smooth mode; stepped must not.
func TestConfiguredPresentationProducesIntermediateWorldFrames(t *testing.T) {
	c, f := viewerFilm(t)
	type result struct {
		FPS, Scale, Frames, OperationFreeFrames, ChangedOperationFreeFrames int
		Mode                                                                string
	}
	var report []result
	for _, fps := range []int{30, 60} {
		for _, scale := range []int{1, 2} {
			for _, mode := range []string{"smooth", "stepped"} {
				c.Runtime.PresentationFPS = fps
				c.Runtime.PresentationMode = mode
				c.Runtime.SimulationRenderScale = scale
				openViewer(t, c, f)
				v := c.Viewer
				p := v.Playback.Preview()
				r := result{FPS: fps, Scale: scale, Mode: mode}
				previous := append([]byte(nil), c.replayFrame(false)...)
				for host := 0; host < fps/2; host++ {
					ordinal := v.Playback.Cursor
					if err := c.AdvanceReplay(1 / float64(fps)); err != nil {
						t.Fatal(err)
					}
					if c.Screen != ScreenReplay {
						t.Fatal("short motion probe prematurely ended")
					}
					before := p.FilmCheckpoint()
					frame := append([]byte(nil), c.replayFrame(false)...)
					if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
						t.Fatal("presentation changed logical state")
					}
					r.Frames++
					if ordinal == v.Playback.Cursor {
						r.OperationFreeFrames++
						changed := false
						w := 320 * scale
						for y := 3 * scale; y < 164*scale && !changed; y++ {
							if !bytes.Equal(previous[y*w+2*scale:y*w+226*scale], frame[y*w+2*scale:y*w+226*scale]) {
								changed = true
							}
						}
						if changed {
							r.ChangedOperationFreeFrames++
						}
					}
					previous = frame
				}
				if r.OperationFreeFrames == 0 || (mode == "smooth" && r.ChangedOperationFreeFrames == 0) || (mode == "stepped" && r.ChangedOperationFreeFrames != 0) {
					t.Fatalf("intermediate world motion mismatch %+v", r)
				}
				report = append(report, r)
			}
		}
	}
	if name := os.Getenv("GS2000_O1_PRESENTATION_METRICS"); name != "" {
		b, _ := json.MarshalIndent(map[string]any{"scope": "source-generated native film, no OS window; actual viewport indexed-frame comparison", "results": report}, "", "  ")
		f, e := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.Write(b)
		f.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Log(fmt.Sprint(report))
}
