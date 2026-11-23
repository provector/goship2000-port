package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"testing"
)

// Real source chooser/save/BEGIN. The existing P6 helper may stop once its one
// selected target dies, so it does not prove the complete advertised allowance.
func artilleryController(t *testing.T, batteryOrdinal int) *FrontEnd {
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
	index, ordinal := -1, 0
	for i, o := range c.Editor.Options {
		if o.Cost == 3 {
			if ordinal == batteryOrdinal {
				index = i
				break
			}
			ordinal++
		}
	}
	if index < 0 {
		t.Fatal("actual source battery absent")
	}
	t.Logf("actual source battery%d %s formation%d component%d template%d", batteryOrdinal, c.Editor.Options[index].Name, c.Editor.Options[index].Formation, c.Editor.Options[index].Component, c.Editor.Options[index].Template)
	a := c.Editor.popupArea(index, len(c.Editor.Options))
	editorClick(t, c, a[0]+2, a[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	editorKey(t, c, "b")
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("ordinary authored BEGIN", c.Screen, c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering.Initialized.Support == nil || c.Rendering.Initialized.Support.Availability != 0x31 {
		t.Fatal("actual initial source AA4", c.Rendering.Initialized.Support)
	}
	return c
}

func TestBothArtilleryCompleteAllowanceAndStrictFilm(t *testing.T) {
	for _, battery := range []int{0, 1} {
		t.Run([]string{"first-source-battery", "second-source-battery"}[battery], func(t *testing.T) {
			c := artilleryController(t, battery)
			p := c.Rendering
			s := p.Initialized.Support
			before := filmJSON(c.Store.State().SelectedPilot())
			target := -1
			for row, e := range p.Initialized.Entities {
				if row >= 9 && e.Kind >= 2 && e.Kind < 0x19 && int8(e.Object.Raw[31]) >= 0 && int8(e.Object.Raw[31]) <= 1 && int16(binary.LittleEndian.Uint16(e.Object.Raw[27:])) > 0 {
					target = row
					break
				}
			}
			if target < 0 {
				t.Fatal("actual enemy target absent")
			}
			e := p.Initialized.Entities[target]
			x, y := navigation.HighPixel(navigation.Point{H0: e.Pose.H0, H1: e.Pose.H1})
			var perRequest []map[string]any
			for use := 0; use < 3; use++ {
				wantBefore := byte(0x31 - use*0x10)
				if s.Request != 0 || s.Availability != wantBefore {
					t.Fatal("previous request did not finish/availability changed", use, s)
				}
				editorAction(t, c, Actions{})
				editorAction(t, c, Actions{GPSOpen: true})
				if p.GPS == nil || !p.GPS.SupportAvailable() {
					t.Fatal("M unavailable after completed prior request", use, p.GPS)
				}
				nativeM(t, c)
				if use == 0 {
					editorAction(t, c, Actions{Selector2: true, PointerPressed: true, Inside: true, X: x, Y: y})
					if s.Request != 0 || s.Availability != 0x31 {
						t.Fatal("cancel spent a request")
					}
					editorAction(t, c, Actions{})
					nativeM(t, c)
					// A confirmed point can still be changed/cancelled within the
					// same GPS entry. EntryRequest is the entry latch, not AA2's
					// later value. No use is charged until leaving that entry.
					editorAction(t, c, Actions{Selector1: true, PointerPressed: true, Inside: true, X: x, Y: y})
					if s.Request != 255 || s.Availability != 0x31 || p.GPS.SupportStrike == nil {
						t.Fatal("provisional confirmed point lost source ownership")
					}
					editorAction(t, c, Actions{})
					var device windowInput
					editorAction(t, c, device.Read(Buttons{GPSKey: sampleGPSCommandKey(func(k int32) bool { return k == 'M' })}))
					if !p.GPS.Support.Editing {
						t.Fatal("entry latch did not allow cancelling same-entry confirmation")
					}
					editorAction(t, c, Actions{Selector2: true, PointerPressed: true, Inside: true, X: x, Y: y})
					if s.Request != 0 || s.Availability != 0x31 || p.GPS.SupportStrike != nil {
						t.Fatal("same-entry cancellation spent allowance or retained strike")
					}
					editorAction(t, c, Actions{})
					nativeM(t, c)
				}
				// Each point is a genuine map selection, including a different point
				// for the second/third request; dead targets do not truncate the test.
				px, py := x+use*2, y-use*2
				editorAction(t, c, Actions{Selector1: true, PointerPressed: true, Inside: true, X: px, Y: py})
				if s.Request != 255 {
					t.Fatal("actual request did not arm", use, s)
				}
				editorAction(t, c, Actions{ViewKey: 1})
				wantAfter := byte(0)
				if use < 2 {
					wantAfter = wantBefore - 0x10
				}
				if s.Availability != wantAfter {
					t.Fatal("request availability not spent once", use, s.Availability, wantAfter)
				}
				// Reentering an active request is source-disabled, and exiting that
				// modal cannot charge another use. Flight is frozen inside GPS.
				editorAction(t, c, Actions{})
				editorAction(t, c, Actions{GPSOpen: true})
				if p.GPS == nil || p.GPS.SupportAvailable() || p.GPS.Support.EntryRequest != 255 {
					t.Fatal("pending request admission", use, p.GPS)
				}
				editorAction(t, c, Actions{GPSKey: "m"})
				if p.GPS.Support.Editing {
					t.Fatal("pending request allowed a second active strike")
				}
				editorAction(t, c, Actions{ViewKey: 1})
				if s.Availability != wantAfter {
					t.Fatal("reentry spent twice")
				}
				shots, impacts, ticks := 0, 0, 0
				p.Flight.CombatEvents = nil
				for ; ticks < 2000 && s.Request != 0; ticks++ {
					if err := c.advanceFlight(.1); err != nil {
						t.Fatal(err)
					}
					if c.Screen == ScreenQuit {
						editorAction(t, c, Actions{Cancel: true})
					}
					for _, ev := range p.Flight.CombatEvents {
						if ev.Kind == "support-launch" {
							shots++
						}
						if ev.Kind == "damage" {
							impacts++
						}
					}
					p.Flight.CombatEvents = nil // observation only, not checkpoint state
				}
				if s.Request != 0 || shots == 0 {
					t.Fatal("strike never completed actual source request", use, ticks, shots, s)
				}
				perRequest = append(perRequest, map[string]any{"use": use + 1, "point": s.Target, "availability_before": wantBefore, "availability_after": s.Availability, "steps": ticks, "projectile_events": shots, "damage_events": impacts})
				t.Logf("battery%d request%d actual point%+v request0 after%d visits; availability%02x→%02x shots%d damage%d", battery, use+1, s.Target, ticks, wantBefore, s.Availability, shots, impacts)
			}
			editorAction(t, c, Actions{})
			editorAction(t, c, Actions{GPSOpen: true})
			if p.GPS.SupportAvailable() {
				t.Fatal("fourth artillery request advertised")
			}
			frozen := filmJSON(s)
			rng := p.Initialized.RandomState
			editorAction(t, c, Actions{GPSKey: "m"})
			if p.GPS.Support.Editing || !bytes.Equal(frozen, filmJSON(s)) || rng != p.Initialized.RandomState {
				t.Fatal("exhausted M mutated source state")
			}
			editorAction(t, c, Actions{ViewKey: 1})
			editorAction(t, c, Actions{EndMission: true})
			editorAction(t, c, Actions{ConfirmQuit: true})
			if c.Screen != ScreenDebrief || !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
				t.Fatal("ordinary pending career", c.Screen, c.Message)
			}
			for n := 0; n < 64 && c.Screen != ScreenFilms; n++ {
				editorAction(t, c, Actions{Enter: true})
				editorAction(t, c, Actions{MenuEnd: true})
				editorAction(t, c, Actions{Enter: true})
			}
			if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
				t.Fatal("ordinary result/finalack/film", c.Screen, c.Message, c.filmError)
			}
			film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if battery == 0 {
				assertSupportResumeIsolation(t, c, film)
			}
			for _, scale := range []int{1, 2} {
				cfg := c.Runtime
				cfg.SimulationRenderScale = scale
				q, er := NewFilmPlayback(c.Office.Assets, film, cfg)
				if er != nil {
					t.Fatal(er)
				}
				for !q.Done {
					if er = q.Step(); er != nil {
						t.Fatal(er)
					}
				}
				if q.Engine.Store != nil {
					t.Fatal("replay acquired career")
				}
			}
			if out := os.Getenv("GS2000_P7_SUPPORT_ARTIFACTS"); out != "" {
				out = filepath.Join(out, []string{"battery0", "battery1"}[battery])
				if err = os.Mkdir(out, 0700); err != nil {
					t.Fatal(err)
				}
				raw, er := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
				if er != nil {
					t.Fatal(er)
				}
				if er = os.WriteFile(filepath.Join(out, "complete-allowance.g2r"), raw, 0600); er != nil {
					t.Fatal(er)
				}
				report, _ := json.MarshalIndent(map[string]any{"configuration": s.Configuration, "requests": perRequest, "header": film.Header, "end": film.End, "operations": len(film.Operations), "scope": "Real production controller author/save/BEGIN/GPS/flight commands and effects; ordinary voluntary end. Not physical OS input or naturally flown objective completion."}, "", "  ")
				if er = os.WriteFile(filepath.Join(out, "complete-allowance.json"), report, 0600); er != nil {
					t.Fatal(er)
				}
			}
		})
	}
}

func assertSupportResumeIsolation(t *testing.T, c *FrontEnd, f *replay.Film) {
	t.Helper()
	first := -1
	for i, o := range f.Operations {
		if o.Kind != "boundary" {
			continue
		}
		var b filmBoundary
		if err := json.Unmarshal(o.Payload, &b); err != nil {
			t.Fatal(err)
		}
		if b.Kind == "gps-support" && b.Support != nil && !b.Support.Cancel {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("complete film lacks actual Support confirmation")
	}
	frozen := filmJSON(f)
	for _, after := range []bool{false, true} {
		q, err := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
		if err != nil {
			t.Fatal(err)
		}
		stop := first
		if after {
			stop++
		}
		for q.Cursor < stop {
			if err = q.Step(); err != nil {
				t.Fatal(err)
			}
		}
		p := q.Preview()
		before := filmJSON(p.Initialized)
		steps := p.Flight.Steps
		rng := p.Initialized.RandomState
		if err = q.Resume(); err != nil {
			t.Fatal(err)
		}
		if q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil || !bytes.Equal(before, filmJSON(p.Initialized)) || p.Flight.Steps != steps || p.Initialized.RandomState != rng {
			t.Fatal("verified Support Resume changed mechanical/career ownership", after)
		}
		if p.GPS == nil {
			t.Fatal("reached Support modal absent", after)
		}
		if after && p.Initialized.Support.Request != 255 {
			t.Fatal("confirmed practice lost actual command")
		}
		if err = q.HandlePractice(Actions{ViewKey: 1}); err != nil {
			t.Fatal(err)
		}
		for n := 0; n < 4; n++ {
			if err = q.AdvancePractice(.1); err != nil {
				t.Fatal(err)
			}
		}
		if q.Engine.Store != nil || !bytes.Equal(frozen, filmJSON(f)) {
			t.Fatal("practice rewrote source film/career")
		}
	}
}

// Air has source AA4=12: one active request allowance, unlike artillery31.
// The supplied predecessor is a labelled Campaign base approach; seed2's new
// support mission, M command, firing, impact, return and result are ordinary.
func TestAirHasOneRequestAndNoFalseReavailability(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	campaignNaturalBaseEnd(t, c)
	loopAcknowledge(t, c)
	seed := int64(2)
	c.MissionSeed = &seed
	readyMission(t, c)
	if c.Mission.Data.Support == nil || c.Mission.Data.Support.Mode != 2 {
		t.Fatal("actual generated Air source absent")
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if p.Initialized.Support.Availability != 0x12 {
		t.Fatal("actual Air allowance", p.Initialized.Support)
	}
	runAirSupportWithPostReturn(t, c, "p7-generated", true, func() {
		if p.Initialized.Support.Request != 0 || p.Initialized.Support.Availability != 0 {
			t.Fatal("actual Air return/allowance", p.Initialized.Support)
		}
		editorAction(t, c, Actions{})
		editorAction(t, c, Actions{GPSOpen: true})
		if p.GPS == nil || p.GPS.SupportAvailable() || p.GPS.SupportStrike != nil {
			t.Fatal("Air completed request falsely remains available/marked")
		}
		before := filmJSON(p.Initialized.Support)
		rng := p.Initialized.RandomState
		editorAction(t, c, Actions{GPSKey: "m"})
		if p.GPS.Support.Editing || !bytes.Equal(before, filmJSON(p.Initialized.Support)) || rng != p.Initialized.RandomState {
			t.Fatal("exhausted Air M mutated source state")
		}
		editorAction(t, c, Actions{ViewKey: 1})
	})
}
