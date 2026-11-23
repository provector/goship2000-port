package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRetainedExportConditionSaveReopenBegin(t *testing.T) {
	c := controlsEditor(t)
	data := c.editorData()
	source := c.Editor.Document
	opts, err := gamestate.AuthoredOptions(data, source, "secondary", 5)
	if err != nil || len(opts) == 0 {
		t.Fatal(opts, err)
	}
	if err = gamestate.PlaceAuthored(data, source, "secondary", 5, opts[0].Formation, 800, 600); err != nil {
		t.Fatal(err)
	}
	pilot := c.Store.State().SelectedPilot()
	in := gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: pilot.Theatre, Rank: pilot.Rank, PilotName: pilot.Name, Seed: int64(source.Seed)}
	m, err := gamestate.CompileAuthoredMission(data, source, in)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := gamestate.NewAuthoredFromMission(data, m, "Retained conditions", source.Seed)
	if err != nil {
		t.Fatal(err)
	}
	budget, budgetErr := gamestate.AuthoredBudget(data, exported)
	if budgetErr == nil || budget >= 0 {
		t.Fatal("fixture must have flattened display overcount", budget, budgetErr)
	}
	initial, _ := json.Marshal(exported.Initial)
	geometry, _ := json.Marshal(exported.Placements)
	career := c.Store.State()
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	rev, err := c.Files.Store.Save(exported, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(exported, rev); err != nil {
		t.Fatal(err)
	}
	// First X validates the retained carrier despite the diagnostic display
	// budget. Return to EDITOR, then use existing admitted condition controls.
	editorKey(t, c, "x")
	if c.Editor.Stage != "exit" || c.Editor.Status != c.editorAssets().Text["valid"] {
		t.Fatal("retained X admission", c.Editor.Stage, c.Editor.Status)
	}
	editorKey(t, c, "e")
	editorKey(t, c, "v")
	editorKey(t, c, "t")
	editorAction(t, c, Actions{Up: true})
	editorAction(t, c, Actions{Enter: true})
	editorKey(t, c, "w")
	editorKey(t, c, "9")
	editorAction(t, c, Actions{MenuTab: true})
	editorKey(t, c, "270")
	editorAction(t, c, Actions{Enter: true})
	wantConditions := c.Editor.Document.Conditions
	editorKey(t, c, "x")
	if c.Editor.Status != c.editorAssets().Text["valid"] {
		t.Fatal(c.Editor.Status)
	}
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles || c.Editor != nil {
		t.Fatal("retained SAVE did not exit", c.Screen)
	}
	saved, rev, err := c.Files.Store.Load(exported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Conditions != wantConditions {
		t.Fatal("admitted conditions did not persist")
	}
	gotInitial, _ := json.Marshal(saved.Initial)
	gotGeometry, _ := json.Marshal(saved.Placements)
	if !bytes.Equal(initial, gotInitial) || !bytes.Equal(geometry, gotGeometry) {
		t.Fatal("condition SAVE changed inverse display/initial owners")
	}
	if got, err := gamestate.AuthoredBudget(data, saved); err == nil || got != budget {
		t.Fatal("condition SAVE guessed a reconstructed budget", got, err)
	}
	if !reflect.DeepEqual(career, c.Store.State()) {
		t.Fatal("condition SAVE wrote career")
	}
	path := filepath.Join(c.Files.Store.Directory, saved.ID+".g2m")
	savedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"digest", "unknown-active", "target", "path", "dependency", "display-formation", "display-component", "display-template", "wind", "temperature", "flags", "day-night"} {
		t.Run(bad, func(t *testing.T) {
			if err := c.editMissionDocument(saved, rev); err != nil {
				t.Fatal(err)
			}
			d := c.Editor.Document
			d.Compiled = nil
			switch bad {
			case "digest":
				d.Initial.Digest = "invalid"
			case "unknown-active":
				d.Initial.Layout.Objects[9].Raw[10] = 0xff
				refreshRetainedDigest(d)
			case "target":
				d.Initial.Layout.Targets.Records[3][32] = 0
				refreshRetainedDigest(d)
			case "path":
				d.Initial.Layout.Paths[0].Flag |= 0x80
				refreshRetainedDigest(d)
			case "dependency":
				d.Dependency = "0000000000000000000000000000000000000000000000000000000000000000"
			case "display-formation":
				d.Placements[0].Formation = 255
			case "display-component":
				d.Placements[0].Component = 8191
			case "display-template":
				d.Placements[0].Template = 65535
			case "wind":
				d.Conditions.WindSpeed = 31
			case "temperature":
				d.Conditions.Temperature = 127
			case "flags":
				d.Conditions.Flags |= 0x400
			case "day-night":
				d.Conditions.Flags ^= 0x200
				w, _ := gamestate.AuthoredWorld(data, d.World)
				at := 25
				if d.Conditions.Flags&0x200 != 0 {
					at = 26
				}
				d.Conditions.Temperature = int8(w.THHD[at])
			}
			before, _ := json.Marshal(d)
			editorKey(t, c, "x")
			editorKey(t, c, "s")
			if c.Screen != ScreenMissionEditor || c.Editor.Stage != "exit" || c.Editor.Status == c.editorAssets().Text["valid"] {
				t.Fatal("invalid retained SAVE admitted", c.Editor.Status)
			}
			after, _ := json.Marshal(d)
			disk, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) || !bytes.Equal(savedBytes, disk) {
				t.Fatal("failed SAVE changed draft or prior file")
			}
		})
	}
	// Reopen through the ordinary Mission Files consumer, then BEGIN with the
	// current posted pilot/actual known crew; no film PilotID is adopted.
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	if err = c.refreshMissionFiles(saved.ID); err != nil {
		t.Fatal(err)
	}
	if err = c.activateMissionFile(19); err != nil {
		t.Fatal(err)
	}
	if c.Editor == nil {
		t.Fatal("reopened EDIT failed")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "e")
	c.Editor = nil
	c.Screen = ScreenMissionFiles
	if err = c.activateMissionFile(15); err != nil {
		t.Fatal(err)
	}
	if c.Mission == nil || c.Mission.PilotID != pilot.ID || c.Mission.Data.Inputs.PilotName != pilot.Name {
		t.Fatal("BEGIN lost current pilot admission", c.Files.Status)
	}
	actual := c.Mission.Data
	if !reflect.DeepEqual(actual.Objects, m.Objects) || !reflect.DeepEqual(actual.Targets, m.Targets) || !reflect.DeepEqual(actual.Objectives, m.Objectives) || !reflect.DeepEqual(actual.Paths, m.Paths) || actual.RawE278 != m.RawE278 || actual.FlightTemplate != m.FlightTemplate || actual.Scene != m.Scene {
		t.Fatal("BEGIN changed immutable launch owners")
	}
	if actual.Conditions.Flags != wantConditions.Flags || actual.Conditions.Temperature != wantConditions.Temperature || actual.Conditions.WindSpeed != wantConditions.WindSpeed || actual.Conditions.WindDirection != wantConditions.WindDirection || actual.Conditions.Hour != m.Conditions.Hour || actual.Conditions.Operation != m.Conditions.Operation {
		t.Fatal("BEGIN lost allowed/frozen conditions")
	}
	if !reflect.DeepEqual(career, c.Store.State()) {
		t.Fatal("reopen/BEGIN preparation wrote career")
	}
}

func refreshRetainedDigest(d *gamestate.AuthoredDocument) {
	h := sha256.New()
	for _, o := range d.Initial.Layout.Objects {
		h.Write(o.Raw[:])
	}
	d.Initial.Layout.ObjectHash = hex.EncodeToString(h.Sum(nil))
	b, _ := json.Marshal(d.Initial.Layout)
	sum := sha256.Sum256(b)
	d.Initial.Digest = hex.EncodeToString(sum[:])
}
