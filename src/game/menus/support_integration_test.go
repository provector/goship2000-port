package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Production UI and source compiler/prefix -> real GPS modal -> artillery slots
// and ordinary impact/damage/credit -> ordinary voluntary result/finalack/film.
// Ground waiting is deliberate; this is not continuous flown objectives or an
// original-runtime/human audio witness.
func TestAuthoredArtilleryGPSRecordingAndResultIsolation(t *testing.T) {
	runAuthoredArtilleryGPSRecording(t, 0, false)
}

// P6 extends the same real lifecycle with physical-key sampler and cancellation
// coverage. The historical first-battery assertions and mechanics are retained.
func runAuthoredArtilleryGPSRecording(t *testing.T, batteryOrdinal int, nativeKey bool) {
	t.Helper()
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	saved, rev, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(saved, rev); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "u")
	battery, ordinal := -1, 0
	for i, o := range c.Editor.Options {
		if o.Cost == 3 {
			if ordinal == batteryOrdinal {
				battery = i
				break
			}
			ordinal++
		}
	}
	if battery < 0 {
		t.Fatal("no battery chooser")
	}
	area := c.Editor.popupArea(battery, len(c.Editor.Options))
	editorClick(t, c, area[0]+2, area[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("SAVE", c.Editor.Status)
	}
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("BEGIN", c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if p.Initialized.Support == nil || p.Initialized.Stores.Definitions[0].CompactID != 2 || p.Initialized.Stores.Definitions[0].RawWSHDID < 25 {
		t.Fatal("real prefix absent")
	}
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	// Choose an actual unarmored ordinary enemy from this realized mission.
	target := -1
	threshold := int16(32767)
	for row, e := range p.Initialized.Entities {
		armor := int8(e.Object.Raw[31])
		limit := int16(binary.LittleEndian.Uint16(e.Object.Raw[29:]))
		score := int16(binary.LittleEndian.Uint16(e.Object.Raw[27:]))
		if row >= 9 && e.Kind >= 2 && e.Kind < 0x19 && armor >= 0 && armor <= 1 && score > 0 && limit > 0 && limit < threshold {
			target, threshold = row, limit
		}
	}
	if target < 0 {
		t.Fatal("no actual static enemy")
	}
	targetPose := p.Initialized.Entities[target].Pose
	x, y := navigation.HighPixel(navigation.Point{H0: targetPose.H0, H1: targetPose.H1})
	launches, damage, destructions := 0, 0, 0
	for request := 0; request < 3; request++ {
		editorAction(t, c, Actions{GPSOpen: true})
		if p.GPS == nil {
			t.Fatal("GPS unavailable")
		}
		if nativeKey {
			nativeM(t, c)
			if request == 0 {
				available := p.Initialized.Support.Availability
				editorAction(t, c, Actions{Selector2: true, PointerPressed: true, Inside: true, X: x, Y: y})
				if p.Initialized.Support.Request != 0 || p.Initialized.Support.Availability != available || p.GPS.Support.Editing {
					t.Fatal("cancel changed actual availability/request")
				}
				editorAction(t, c, Actions{})
				nativeM(t, c)
			}
		} else {
			editorAction(t, c, Actions{GPSKey: "m"})
		}
		if p.GPS.Support == nil || !p.GPS.Support.Editing {
			t.Fatal("M didnotarmrealSupport", p.GPS.Support)
		}
		editorAction(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
		if p.Initialized.Support.Request != 255 {
			t.Fatal("GPSrequest not consumed", p.Initialized.Support)
		}
		editorAction(t, c, Actions{ViewKey: 1})
		for tick := 0; tick < 700; tick++ {
			if err = c.advanceFlight(.1); err != nil {
				t.Fatal(err)
			}
			if c.Screen == ScreenQuit {
				editorAction(t, c, Actions{Cancel: true})
			}
			for _, e := range p.Flight.Combat.Events {
				if e.Kind == "support-launch" {
					launches++
				}
				if e.Kind == "damage" {
					damage++
				}
				if e.Kind == "destroyed" {
					destructions++
				}
			}
			if p.Initialized.Support.Request == 0 {
				break
			}
		}
		if p.Initialized.Entities[target].Participation != 1 {
			break
		}
	}
	// Outcome is the actual damage/credit owner, never precompleted goals/poses.
	// The normal transport drains Events; retained current state and result log
	// provide independent receipts even if no shell lands in the selected proxy.
	accumulated := int16(binary.LittleEndian.Uint16(p.Initialized.Entities[target].DynamicRaw[4:]))
	credit := p.Flight.Combat.Credit[target/2]
	if target&1 != 0 {
		credit >>= 4
	}
	credit &= 15
	if credit != 7 {
		t.Fatalf("real Support did not produce sourceexternalcredit7: target%d/damage%d/credit%d", target, accumulated, credit)
	}
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal("result", c.Screen, c.Message)
	}
	staged, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, staged) {
		t.Fatal("Support committed beforefinalack")
	}
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("film/finalack", c.Screen, c.Message, c.filmError)
	}
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if nativeKey {
		if film.Header.InputSchema != replay.R1InputCompatibility {
			t.Fatal("new Support recording lacks explicit v6 input", film.Header.InputSchema)
		}
		wrong := *film
		wrong.Header = film.Header
		wrong.Header.InputSchema = replay.P4InputCompatibility
		if film.P6Mechanics() {
			if _, err := NewFilmPlayback(c.Office.Assets, &wrong, c.Runtime); err == nil {
				t.Fatal("new P6 simulation admitted its unrecorded v4 input pairing")
			}
			// Deliberately misinterpret only this temporary no-Train test film
			// under the existing P5/v4 pair to discriminate reached GPS policy.
			// The immutable source file/header and historical files stay intact.
			wrong.Header.Simulation = replay.P5AuthoredCompatibility
		}
		legacy, e := NewFilmPlayback(c.Office.Assets, &wrong, c.Runtime)
		if film.R1Mechanics() {
			// R1's new recorded route owner cannot be reinterpreted as the
			// P5/v4 stream. Its strict checkpoint domain rejects before the
			// historical v5-Space interaction exercised by earlier profiles.
			if e == nil || !strings.Contains(e.Error(), "checkpoint owner") {
				t.Fatal("R1 route owner admitted historically", e)
			}
		} else if e != nil {
			t.Fatal("v4 pairing must remain admitted before reached-mode divergence", e)
		} else {
			var divergence error
			for !legacy.Done && divergence == nil {
				divergence = legacy.Step()
			}
			if divergence == nil {
				t.Fatal("v5 Space/mode stream incorrectly verified under historical v4")
			}
		}
	}
	for _, scale := range []int{1, 2} {
		cfg := c.Runtime
		cfg.SimulationRenderScale = scale
		playback, e := NewFilmPlayback(c.Office.Assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !playback.Done {
			if e = playback.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if playback.Engine.Store != nil {
			t.Fatal("Support replay acquired career")
		}
	}
	out := os.Getenv("GS2000_P4_EDITOR_ARTIFACTS")
	if nativeKey && os.Getenv("GS2000_P6_SUPPORT_ARTIFACTS") != "" {
		out = filepath.Join(os.Getenv("GS2000_P6_SUPPORT_ARTIFACTS"), fmt.Sprintf("artillery-%d", batteryOrdinal))
		if err := os.Mkdir(out, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if out != "" {
		raw, e := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "authored-artillery.g2r"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		report, _ := json.MarshalIndent(map[string]any{"film": film.Header, "operations": len(film.Operations), "end": film.End, "target_row": target, "actual_target_pose": targetPose, "target_damage": accumulated, "credit": credit, "support": p.Initialized.Support, "scope": "normal author/save/BEGIN/briefing/loadout and real GPS/artillery; ordinary voluntary end. Not human flown objective completion or original runtime"}, "", "  ")
		if e = os.WriteFile(filepath.Join(out, "artillery-recording.json"), report, 0600); e != nil {
			t.Fatal(e)
		}
	}
	_ = gamestate.SupportInitializationProfile
}
