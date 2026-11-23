package menus

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gunship2000-assets/game/replay"
	state "gunship2000-assets/game/state"
)

// Real controller paths using a newly authored test mission. Every iteration
// reopens the same career and starts another sortie from its durable state.
func TestPortableSortieCycles(t *testing.T) {
	c := authoredUIController(t)
	c.simulationProfile = ""
	mission := regressionDocument(t, c, "Cycle pickup", "EURO6.WRL")
	_, err := state.CompileAuthoredMission(c.editorData(), mission, state.MissionInputs{MissionType: state.MissionSimulated, Theatre: state.TheatreCentralEurope, Rank: state.RankCaptain, PilotName: "Cycle pilot", Seed: int64(mission.Seed)})
	if err != nil {
		t.Fatal(err)
	}
	missions := state.AuthoredStore{Directory: c.MissionDirectory}
	if _, err = missions.Save(mission, ""); err != nil {
		t.Fatal(err)
	}
	dir := c.Store.Directory
	films := c.FilmDirectory
	for i := 0; i < 5; i++ {
		s, err := state.OpenSaveStore(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		front, err := NewFrontEnd(c.Office.Assets, s)
		if err != nil {
			t.Fatal(err)
		}
		front.MissionDirectory = missions.Directory
		front.FilmDirectory = films
		if err = front.OpenMissionFiles(); err != nil {
			t.Fatal(err)
		}
		editorKey(t, front, "b")
		for n := 0; n < 100 && front.Screen != ScreenBriefing; n++ {
			if err = front.Tick(); err != nil {
				t.Fatal(err)
			}
		}
		if front.Screen != ScreenBriefing {
			t.Fatal("BEGIN", i, front.Screen, front.Files.Status)
		}
		startRendering(t, front)
		jFinishFade(t, front)
		for n := 0; n < 12; n++ {
			if err = front.advanceFlight(.1); err != nil {
				t.Fatal(err)
			}
		}
		if front.Screen == ScreenQuit {
			front.cancelNaturalSortieEnd()
			front.Screen = ScreenScene
		}
		editorAction(t, front, Actions{EndMission: true})
		editorAction(t, front, Actions{ConfirmQuit: true})
		if front.Screen != ScreenDebrief {
			t.Fatal(i, front.Screen, front.Message)
		}
		id := front.Debrief.Pending.Start.ID
		ackToLibrary(t, front)
		rev := s.State().Revision
		if err = s.CommitSortie(id); err != nil || s.State().Revision != rev {
			t.Fatal("duplicate acknowledgment", i, err)
		}
		if len(front.Library.Entries) != 1 {
			t.Fatalf("cycle %d: expected Last Mission replacement, got %d films", i, len(front.Library.Entries))
		}
		entry := front.Library.Entries[0]
		film, err := front.Library.Store.Load(entry.ID)
		if err != nil {
			t.Fatal(err)
		}
		before := filmJSON(s.State())
		if err = checkSharedPlayback(front, film); err != nil {
			t.Fatal(i, err)
		}
		if !bytes.Equal(before, filmJSON(s.State())) {
			t.Fatal("career changed in playback or practice")
		}
		reopened, err := state.OpenSaveStore(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if got := int(reopened.State().SelectedPilot().Statistics.MissionCampaignCounter); got != 13+i {
			t.Fatalf("restart after cycle %d, counter %d", i, got)
		}
		if i == 4 {
			if out := os.Getenv("GS2000_PUBLIC_CYCLE_EXPORT"); out != "" {
				if err = os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				for name, path := range map[string]string{"Cycle pickup.g2m": filepath.Join(missions.Directory, mission.ID+".g2m"), "Cycle film.g2r": publishedFilmPath(t, front, entry.ID)} {
					b, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(out, name), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		front.AbandonFilmRecording()
		if os.Getenv("GS2000_TEST_CYCLE_REACHABILITY") == "1" {
			runtime.GC()
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf("cycle=%d heap_alloc=%d heap_inuse=%d goroutines=%d revision=%d", i+1, m.HeapAlloc, m.HeapInuse, runtime.NumGoroutine(), rev)
	}
}
func checkSharedPlayback(c *FrontEnd, f *replay.Film) error {
	q, e := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
	if e != nil {
		return e
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			return e
		}
	}
	if q.Divergence != "" {
		return fmt.Errorf("strict playback: %s", q.Divergence)
	}
	q, e = NewFilmPlayback(c.Office.Assets, f, c.Runtime)
	if e != nil {
		return e
	}
	if e = q.Step(); e != nil {
		return e
	}
	if e = q.Resume(); e != nil {
		return e
	}
	if q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
		return fmt.Errorf("practice acquired career")
	}
	return q.AdvancePractice(.1)
}
func TestReceivedEditorMissionAndFilm(t *testing.T) {
	input := os.Getenv("GS2000_RECEIVED_CYCLE")
	if input == "" {
		t.Skip("requires a peer's generated W2 cycle pair")
	}
	c := authoredUIController(t)
	before := filmJSON(c.Store.State())
	for name, dest := range map[string]string{"Cycle pickup.g2m": c.MissionDirectory, "Cycle film.g2r": c.FilmDirectory} {
		b, e := os.ReadFile(filepath.Join(input, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(dest, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dest, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	store := replay.Store{Root: c.FilmDirectory}
	entries, e := store.List()
	if e != nil || len(entries) != 1 {
		t.Fatal("library discovery", e)
	}
	f, e := store.LoadEntry(entries[0])
	if e != nil {
		t.Fatal(e)
	}
	if e = checkSharedPlayback(c, f); e != nil {
		t.Fatal(e)
	}
	if e = c.OpenMissionFiles(); e != nil {
		t.Fatal(e)
	}
	editorKey(t, c, "e")
	if c.Editor == nil || c.Editor.Document.Title != "Cycle pickup" {
		t.Fatal("imported mission", c.Files.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal(c.Screen)
	}
	if !bytes.Equal(before, filmJSON(c.Store.State())) {
		t.Fatal("receiver career changed")
	}
	t.Logf("film %s %q, mission %s %q", f.Header.ID, entries[0].Title, c.Files.Entries[0].ID, c.Files.Entries[0].Title)
}
