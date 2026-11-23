package menus

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
)

// Exercise the actual authored UI/BEGIN, reached source base prompt, Handle N,
// immediately following reference frame, keyboard GPS and ordinary ending.
// No pose, clock/RNG, pending/result flag or checkpoint is supplied.
func TestNaturalEndCancellationRecordedBeforeReferenceAndGPS(t *testing.T) {
	for _, cancel := range []bool{true, false} {
		t.Run(map[bool]string{true: "N-reference-GPS-AltE", false: "natural-Y"}[cancel], func(t *testing.T) {
			c := authoredUIController(t)
			authorThroughUI(t, c)
			editorKey(t, c, "b")
			for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
				if e := c.Tick(); e != nil {
					t.Fatal(e)
				}
			}
			if c.Screen != ScreenBriefing {
				t.Fatal("ordinary BEGIN missing", c.Screen, c.Message)
			}
			startRendering(t, c)
			jFinishFade(t, c)
			p := c.Rendering
			before := filmJSON(c.Store.State().SelectedPilot())
			for i := 0; i < 500 && c.Screen == ScreenScene; i++ {
				if e := c.AdvanceRendering(.12); e != nil {
					t.Fatal(e)
				}
			}
			if c.Screen != ScreenQuit || !c.sceneEndConfirmation || p.Initialized.Sortie == nil || p.Initialized.Sortie.Pending == "" {
				t.Fatal("real natural prompt not reached", c.Screen, p.Initialized.Sortie)
			}
			if cancel {
				if e := c.Handle(Actions{Text: []rune{'n'}, AnyKey: true}); e != nil {
					t.Fatal(e)
				}
				if c.Screen != ScreenScene || p.Initialized.Sortie.Pending != "" || p.Initialized.Sortie.Status&0x80 != 0 || p.Flight.State.FlightStage != 128 {
					t.Fatal("source N transaction missing")
				}
				n := p.filmRecorder.operationCount()
				c.cancelNaturalSortieEnd()
				if p.filmRecorder.operationCount() != n {
					t.Fatal("duplicate N recorded without a pending prompt")
				}
				if e := c.StepRendering(1); e != nil {
					t.Fatal(e)
				}
				if e := c.Tick(); e != nil {
					t.Fatal(e)
				}
				editorAction(t, c, Actions{GPSOpen: true})
				editorAction(t, c, Actions{GPSKey: "f"})
				editorAction(t, c, Actions{SceneDirections: [4]bool{false, true, false, false}})
				for i := 0; i < 30; i++ {
					if e := c.Tick(); e != nil {
						t.Fatal(e)
					}
				}
				editorAction(t, c, Actions{GPSKey: "enter"})
				editorAction(t, c, Actions{GPSKey: "f"})
				editorAction(t, c, Actions{ViewKey: 1})
				editorAction(t, c, Actions{EndMission: true})
				editorAction(t, c, Actions{ConfirmQuit: true})
			} else {
				editorAction(t, c, Actions{ConfirmQuit: true})
			}
			if c.Screen != ScreenDebrief || !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
				t.Fatal("ending/finalack ownership", c.Screen)
			}
			sealed, e := c.SealRecordedFilm()
			if e != nil || sealed == nil {
				t.Fatal(e)
			}
			var cancels, ends int
			for _, op := range sealed.Operations {
				if op.Kind == "end" {
					ends++
				}
				if op.Kind == "boundary" {
					var b filmBoundary
					_ = json.Unmarshal(op.Payload, &b)
					if b.Kind == "natural-end-cancel" {
						cancels++
					}
				}
			}
			want := 0
			if cancel {
				want = 1
			}
			if cancels != want || ends != 1 || sealed.Header.InputSchema != replay.R1InputCompatibility {
				t.Fatal("wrong explicit transaction count/profile", cancels, ends, sealed.Header)
			}
			id := sealed.Header.ID
			loopAcknowledge(t, c)
			f, e := (replay.Store{Root: c.FilmDirectory}).Load(id)
			if e != nil {
				t.Fatal(e)
			}
			for _, scale := range []int{1, 2} {
				opt := configuration.Defaults()
				opt.SimulationRenderScale = scale
				q, e := NewFilmPlayback(c.Office.Assets, f, opt)
				if e != nil {
					t.Fatal(e)
				}
				for !q.Done {
					if e = q.Step(); e != nil {
						t.Fatal("new recorded production sequence", scale, e)
					}
				}
				if q.Engine.Store != nil {
					t.Fatal("playback acquired career writer")
				}
			}
			if cancel {
				b, e := replay.Marshal(f)
				if e != nil {
					t.Fatal(e)
				}
				old, e := replay.Unmarshal(b)
				if e != nil {
					t.Fatal(e)
				}
				// Isolate the older cancellation predicate from Q31's separately
				// rejected held-jammer operand; no historical film is rewritten.
				for i := range old.Operations {
					op := &old.Operations[i]
					if op.Kind == "step" || op.Kind == "terminal" {
						var v filmStep
						if err := decodeFilmPayload(op.Payload, &v); err != nil {
							t.Fatal(err)
						}
						v.Input.Jammers = nil
						op.Payload = filmJSON(v)
					}
				}
				old.Header.InputSchema = replay.PreviousInputCompatibility
				if e = validateFilmOperations(old); e == nil || !strings.Contains(e.Error(), "cancellation") {
					t.Fatal("v3 transaction admitted under historical v2", e)
				}
				for i := range f.Operations {
					op := &f.Operations[i]
					if op.Kind != "boundary" {
						continue
					}
					var boundary filmBoundary
					_ = json.Unmarshal(op.Payload, &boundary)
					if boundary.Kind != "natural-end-cancel" {
						continue
					}
					boundary.Action.Enter = true
					op.Payload = filmJSON(boundary)
					if e = validateFilmOperations(f); e == nil {
						t.Fatal("cancel accepted unrelated carrier")
					}
					break
				}
			}
		})
	}
}

func TestRecordedMissingNaturalCancelRemainsStrictlyGuarded(t *testing.T) {
	path := os.Getenv("GS2000_P2_FAILED_CANCEL_FILM")
	if path == "" {
		t.Skip("explicit preserved connected failed film required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if assetdata.Hash(raw) != "0b07d451acb067f97d3197cce7e1eb4df5ea61f14cf5c9c9e210c9a14c3eded5" {
		t.Fatal("wrong connected failure fixture")
	}
	f, e := replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	if f.Header.InputSchema != replay.PreviousInputCompatibility {
		t.Fatal("historical input profile changed")
	}
	c := authoredUIController(t)
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			break
		}
	}
	if e == nil || q.Cursor != 461 || q.FailureDiagnostic == nil || q.FailureDiagnostic.ReachedSteps != 211 || q.FailureDiagnostic.Clock.Phase != 128 {
		t.Fatal("failed historical film was repaired or diverged elsewhere", e, q.Cursor, q.FailureDiagnostic)
	}
	var groups []string
	for k, v := range q.FailureDiagnostic.Expected {
		if q.FailureDiagnostic.Actual[k] != v {
			groups = append(groups, k)
		}
	}
	if len(groups) != 2 || q.FailureDiagnostic.Expected["flight"] == q.FailureDiagnostic.Actual["flight"] || q.FailureDiagnostic.Expected["mission"] == q.FailureDiagnostic.Actual["mission"] {
		t.Fatal("historical guarded difference changed", groups)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("protected failure changed", e)
	}
}
