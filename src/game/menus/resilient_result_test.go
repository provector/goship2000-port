package menus

import (
	"archive/zip"
	"gunship2000-assets/game/replay"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flightcontrol"
)

// Start from an admitted generated Campaign. Only the failing route index is
// supplied. The actor dispatcher, outcome, debrief and commit remain real.
func TestRecoverableWaypointCommitsAndReports(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	s, err := diagnostics.StartSession(c.Store.Directory, "test build", "ordinary logging off")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	r := c.Rendering
	t.Logf("participating aircraft %d", len(r.Initialized.Aircraft))
	if len(r.Initialized.Aircraft) != 5 {
		t.Fatal("fixture needs full section")
	}
	e := &r.Initialized.Entities[1]
	e.Kind, e.Participation, e.ControlRaw[0], e.ControlRaw[1] = 0x23, 1, 0, 3
	e.DynamicRaw[0], e.DynamicRaw[1] = e.Kind, e.Participation
	before := c.Store.State().Revision
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && r.Flight.Combat.RouteWarnings()[1] == ""; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if r.Flight.Combat.RouteWarnings()[1] == "" {
		t.Fatal("dispatcher did not encounter supplied index", e.Kind, e.Participation)
	}
	for _, a := range []Actions{{EndMission: true}, {ConfirmQuit: true}} {
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	if c.Debrief == nil {
		t.Fatal("no debrief", c.Message)
	}
	id := c.Debrief.Pending.Start.ID
	ackToLibrary(t, c)
	rev := c.Store.State().Revision
	if rev <= before {
		t.Fatal("missing commit")
	}
	if err := c.Store.CommitSortie(id); err != nil || c.Store.State().Revision != rev {
		t.Fatal("repeat commit", err)
	}
	s.Close()
	paths, _ := filepath.Glob(filepath.Join(c.Store.Directory, "reports", "session-*", "incident-*", "error_report.zip"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
	z, err := zip.OpenReader(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "report.txt" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(r)
			r.Close()
			if err != nil || !strings.Contains(string(b), "invalid waypoint index") || !strings.Contains(string(b), "Your mission result was saved.") {
				t.Fatal(string(b), err)
			}
			return
		}
	}
	t.Fatal("missing report text")
}

// Capture a new native film through screen handlers. Strict playback must
// reconstruct both the unfinished edit and the handover before practice forks.
func TestRecordedCollectiveAndWingmanDraftResume(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	act := func(a Actions) {
		t.Helper()
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	act(Actions{Cancel: true})
	act(Actions{})
	in := flightcontrol.LiveInput{Mode: "relative_height", Epoch: 1, Slider: true, Lever: 32768}
	in.Bound[2] = true
	for i := 0; i < 7; i++ {
		a := Actions{Live: &in}
		if i == 0 || i == 1 {
			a.FlightCommands = []int{0}
		}
		if i == 5 {
			in.Lever = 46000
		}
		act(a)
		if err := c.AdvanceRendering(.11); err != nil {
			t.Fatal(err)
		}
		if i == 3 && !c.Rendering.Flight.collectiveOwner.Keyboard {
			t.Fatal("parked joystick stole keyboard collective")
		}
	}
	if c.Rendering.Flight.collectiveOwner.Keyboard {
		t.Fatal("deliberate movement did not take ownership")
	}
	act(Actions{GPSOpen: true})
	act(Actions{})
	act(Actions{GPSKey: "n"})
	act(Actions{GPSKey: "f"})
	act(Actions{Selector1: true, Inside: true, X: 120, Y: 85})
	g := c.Rendering.GPS
	if g == nil || g.Selected == 0 || g.ReplayState().Draft == nil {
		t.Fatal("no pending wingman edit")
	}
	act(Actions{ViewKey: 1})
	act(Actions{EndMission: true})
	act(Actions{ConfirmQuit: true})
	if c.Debrief == nil {
		t.Fatal(c.Message)
	}
	ackToLibrary(t, c)
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	verify := func() *FilmPlayback {
		q, e := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
		if e != nil {
			t.Fatal(e)
		}
		return q
	}
	q := verify()
	for !q.Done {
		if e := q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	q = verify()
	for !q.Done {
		if err := q.Step(); err != nil {
			t.Fatal(err)
		}
		g := q.Preview().GPS
		if g != nil && g.ReplayState().Draft != nil {
			old := g.Routes[g.Selected]
			if err := q.Resume(); err != nil {
				t.Fatal(err)
			}
			if err := q.HandlePractice(Actions{ViewKey: 1}); err != nil {
				t.Fatal(err)
			}
			if g.Routes[g.Selected] != old || q.Engine.Store != nil {
				t.Fatal("resume published unfinished route or acquired career")
			}
			return
		}
	}
	t.Fatal("strict replay never reconstructed pending edit")
}

// A killed writer leaves a prefix and session journal, never a result receipt.
func TestUnexpectedSessionArchivesVerifiedPrefix(t *testing.T) {
	if root := os.Getenv("G2_U26_CRASH_FIXTURE"); root != "" {
		session, e := diagnostics.StartSession(root, "child", "logging off")
		if e != nil {
			t.Fatal(e)
		}
		defer session.Close()
		diagnostics.SetMission("fixture", os.Getenv("G2_U26_FILM_ID"), "flight", 1)
		if e := os.WriteFile(filepath.Join(root, "ready"), nil, 0600); e != nil {
			t.Fatal(e)
		}
		select {}
	}
	c := developmentScenarioController(t, "o-short-campaign")
	campaignNaturalBaseEnd(t, c)
	ackToLibrary(t, c)
	id := c.Library.Entries[0].ID
	src := publishedFilmPath(t, c, id)
	blocks, n, valid, _ := replay.InspectPrefix(src)
	if blocks < 2 || n < 1 {
		t.Fatal(blocks, n, valid)
	}
	root := t.TempDir()
	stages := filepath.Join(root, "replays")
	if e := os.MkdirAll(stages, 0700); e != nil {
		t.Fatal(e)
	}
	bytes, e := os.ReadFile(src)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(stages, ".film-"+id+".tmp"), bytes[:valid], 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestUnexpectedSessionArchivesVerifiedPrefix$")
	cmd.Env = append(os.Environ(), "G2_U26_CRASH_FIXTURE="+root, "G2_U26_FILM_ID="+id)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	end := time.Now().Add(10 * time.Second)
	for {
		if _, e = os.Stat(filepath.Join(root, "ready")); e == nil {
			break
		}
		if time.Now().After(end) {
			t.Fatal("no ready marker")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if e = cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	if cmd.Wait() == nil {
		t.Fatal("expected interrupted process")
	}
	session, e := diagnostics.StartSession(root, "next", "both off")
	if e != nil {
		t.Fatal(e)
	}
	session.Close()
	paths, _ := filepath.Glob(filepath.Join(root, "reports", "session-*", "crash_report.zip"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
	z, e := zip.OpenReader(paths[0])
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "mission.g2r" {
			t.Fatal("prefix promoted to complete mission")
		}
		if f.Name == "mission-prefix.g2r" {
			r, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			b, e := io.ReadAll(r)
			r.Close()
			if e != nil || !slices.Equal(b, bytes[:valid]) {
				t.Fatal("saved prefix differs")
			}
			return
		}
	}
	t.Fatal("prefix missing")
}

func TestCloseWhileRejectedCareerWritePending(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	id := c.Mission.ResultLaunch.Start.ID
	before := c.Store.State().Revision
	c.Mission.ResultLaunch.Tables.SourceProfile = "supplied invalid progression source"
	for _, a := range []Actions{{Cancel: true}, {EndMission: true}, {ConfirmQuit: true}} {
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	for c.rejectedCheck != nil && c.rejectedCheck.published == nil {
		if e := c.AdvanceReplay(.016); e != nil {
			t.Fatal(e)
		}
	}
	if c.rejectedCheck == nil {
		t.Fatal("no pending writer")
	}
	if err := c.Handle(Actions{Close: true}); err != nil || !c.rejectedCheck.closeRequested {
		t.Fatal("lost close request", err)
	}
	until := time.Now().Add(10 * time.Second)
	for c.rejectedCheck != nil {
		if time.Now().After(until) {
			t.Fatal("writer did not finish")
		}
		if e := c.AdvanceReplay(.016); e != nil {
			t.Fatal(e)
		}
		time.Sleep(time.Millisecond)
	}
	if c.Screen != ScreenQuit || c.Store.State().Revision != before {
		t.Fatal("close or career ownership", c.Screen)
	}
	if _, e := os.Stat(filepath.Join(c.Store.Directory, "unresolved-results", id+".json")); e != nil {
		t.Fatal(e)
	}
}
