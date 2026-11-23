package menus

import (
	"bytes"
	"fmt"
	"gunship2000-assets/game/render3d"
	"reflect"
	"testing"
)

// These are supplied close render fixtures, not changed launch/film facts.
// Actual original definitions and normal ORDNANCE initialization independently
// establish model59/callback1 and model138/callback2 in the coverage probe.
func TestReplayReachesSharedRotorPresentation(t *testing.T) {
	c, film := viewerFilm(t)
	for _, model := range []struct {
		id       int
		callback uint8
	}{{59, 1}, {138, 2}, {166, 1}} {
		for _, mode := range []string{"stepped", "smooth"} {
			for _, role := range []string{"running-player", "stopped-player", "running-follower"} {
				t.Run(fmt.Sprintf("model%d/%s/%s", model.id, mode, role), func(t *testing.T) {
					openViewer(t, c, film)
					v := c.Viewer
					p := v.Playback.Preview()
					p.presentation.mode = mode
					p.Scene.RenderScale = 1
					p.Flight.State.Terminal = nil
					p.Flight.TerminalPresentation.Active = false
					p.Flight.State.Rotor = 1
					if role != "running-player" {
						p.Flight.State.Rotor = 0
					}
					player := p.Initialized.Entities[p.Initialized.PlayerRow]
					tracked := player
					tracked.ModelID, tracked.Kind, tracked.Participation = model.id, 0, 1
					if role == "running-follower" {
						tracked.ID = "r14-render-only-follower"
						p.Initialized.Entities = append(p.Initialized.Entities, tracked)
						p.Initialized.FlightRows = append(p.Initialized.FlightRows, len(p.Initialized.Entities)-1)
					} else {
						p.Initialized.Entities[p.Initialized.PlayerRow] = tracked
					}
					in := render3d.Instance{ID: tracked.ID, ModelID: model.id, Callback: model.callback, Pose: render3d.Pose{H0: tracked.Pose.H0, H1: tracked.Pose.H1, Height: tracked.Pose.Height}, Angles: tracked.Pose.Angles}
					p.instances = []render3d.Instance{in}
					p.Scene.Instances = []render3d.Instance{in}
					v.previousInstances = append([]render3d.Instance(nil), p.instances...)
					v.camera = render3d.Camera{Pose: render3d.Pose{H0: in.H0, H1: in.H1 - 256, Height: in.Height + 180}, Angles: [3]uint16{0xed00, 0, 0}}
					v.previousCamera = v.camera
					v.alpha = .25
					var frames [][]byte
					for phase := 0; phase < 3; phase++ {
						p.Visual.Phase = uint64(phase)
						before := p.FilmCheckpoint()
						frames = append(frames, append([]byte(nil), c.replayFrame(false)...))
						if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
							t.Fatal("viewer presentation changed a canonical checkpoint")
						}
					}
					equal := bytes.Equal(frames[0], frames[1]) && bytes.Equal(frames[1], frames[2])
					if role == "stopped-player" && !equal {
						t.Fatal("genuinely stopped player rotor animated")
					}
					if role != "stopped-player" && equal {
						t.Fatal("actual replay renderer bypassed native rotor presentation")
					}
				})
			}
		}
	}
}
