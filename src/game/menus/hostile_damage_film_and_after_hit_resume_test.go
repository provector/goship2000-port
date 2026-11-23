package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Normally saved encounter, actual hostile release/contact and gameplay RNG.
// No projectile, selected damage, initial health or result is supplied.
func TestHostileDamageFilmAndAfterHitResume(t *testing.T) {
	c := naturalControllerProfile(t, false, replay.R8AuthoredCompatibility)
	p := c.Rendering
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	step := func() {
		t.Helper()
		if e := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); e != nil {
			t.Fatal(e)
		}
		if p.Flight.Failure != "" || p.Error != nil {
			t.Fatal(p.Flight.Failure, p.Error)
		}
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	hitOperation := 0
	for i := 0; i < 600 && c.Rendering != nil; i++ {
		if i < 32 {
			c.Handle(Actions{FlightCommands: []int{0}})
		} else {
			c.Handle(Actions{})
		}
		step()
		d := p.Flight.Systems.Damage
		if d[1] == 2 || d[6] == 2 {
			hitOperation = p.filmRecorder.operationCount()
			break
		}
	}
	if hitOperation == 0 {
		t.Fatalf("no critical wing in bounded known encounter; damage%v", p.Flight.Systems.Damage)
	}
	for i := 0; i < 5 && c.Rendering != nil; i++ {
		step()
	}
	finishAndReplay(t, c, before, "r8-gun-damage", replay.R8AuthoredCompatibility)
	f, e := c.Library.Store.Load(c.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	current := c.Runtime
	current.DisableEnemyAI = true
	current.DisableWingmenAI = false
	q, e := NewFilmPlayback(c.Office.Assets, f, current)
	if e != nil {
		t.Fatal(e)
	}
	for q.Cursor < hitOperation && !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if q.Done || (!q.Preview().Initialized.Stores.VictimWingStores || !q.Preview().Initialized.Stores.VictimCriticalProficiency || !q.Preview().r8Mechanics) {
		t.Fatal("recorded damage policy lost")
	}
	stores := filmJSON(q.Preview().Initialized.Stores)
	if e = q.Resume(); e != nil || !bytes.Equal(stores, filmJSON(q.Preview().Initialized.Stores)) || q.Engine.Store != nil {
		t.Fatal("after-hit Resume", e)
	}
	if out := os.Getenv("GS2000_R8_OUTPUT"); out != "" {
		raw, e := replay.Marshal(f)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "r8-gun-damage.g2r"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("real gun contact reached wing damage at operation%d; full strict playback, commit/restart and isolated after-hit Resume", hitOperation)
}
