package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualCatalogueEditorPlacementConsumers(t *testing.T) {
	for _, inputs := range []struct {
		name, key string
		phil3     int
	}{
		{"retained-R23-preparation", "GS2000_TEST_R221_ASSETS", 95},
		{"repaired-map", "GS2000_TEST_MISSIONS", 95},
	} {
		t.Run(inputs.name, func(t *testing.T) {
			root := os.Getenv(inputs.key)
			if root == "" {
				t.Fatalf("supply retained preparation via %s", inputs.key)
			}
			t.Setenv("GS2000_TEST_MISSIONS", root)
			verifyCataloguePlacements(t, inputs.phil3)
		})
	}
}
func verifyCataloguePlacements(t *testing.T, expectedPhil3 int) {
	t.Helper()
	c := authoredUIController(t)
	data := c.editorData()
	records := []map[string]any{}
	placed, guarded, limited := 0, 0, 0
	phil3Placements := 0
	for _, w := range data.Worlds {
		if m, ok := c.editorAssets().Maps[editorMapKey(w.WorldName)]; !ok {
			continue
		} else if m.Unavailable != "" || len(m.Pixels) != 64000 {
			records = append(records, map[string]any{"world": w.WorldName, "row_type": "map-precondition", "result": "known-map-decoder-exclusion-not-placement-pass", "reason": m.Unavailable})
			continue
		}
		d, err := gamestate.NewAuthoredDocument(data, w.WorldName, "Catalogue source", 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []struct {
			role       string
			kind, x, y int
		}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
			opts, e := gamestate.AuthoredOptions(data, d, q.role, q.kind)
			if e != nil || len(opts) == 0 {
				t.Fatal(w.WorldName, q, e)
			}
			if e = gamestate.PlaceAuthored(data, d, q.role, q.kind, opts[0].Formation, q.x, q.y); e != nil {
				t.Fatal(w.WorldName, q, e)
			}
		}
		for category := 1; category <= 10; category++ {
			for _, formation := range []bool{false, true} {
				opts, _, e := gamestate.AuthoredEditorOptions(data, d, "defender", 0, category, formation)
				if e != nil {
					t.Fatal(e)
				}
				for index, o := range opts {
					row := map[string]any{"world": w.WorldName, "category": category, "formation_mode": formation, "formation": o.Formation, "component": o.Component, "template": o.Template, "name": o.Name, "option_index": index, "declared_components": o.Cost, "consumer": "editorArm -> editorSelectOption -> handleEditor -> PlaceAuthoredOption", "input_evidence": "native controller; not OS mouse"}
					if index >= 36 {
						limited++
						row["result"] = "outside-source-ui-list-limit"
						records = append(records, row)
						continue
					}
					q, _ := gamestate.CloneAuthored(d)
					if e = c.editMissionDocument(q, ""); e != nil {
						t.Fatal(e)
					}
					c.Editor.Formation = formation
					if e = c.editorArm("defender", 0, category); e != nil {
						t.Fatal(e)
					}
					if index >= len(c.Editor.Options) || c.Editor.Options[index].Formation != o.Formation || c.Editor.Options[index].Component != o.Component {
						t.Fatal("actual menu changed source selection", w.WorldName, category, index)
					}
					if e = c.editorSelectOption(index); e != nil {
						t.Fatal(e)
					}
					if o.Unavailable != "" {
						guarded++
						if c.Editor.Held || c.Editor.Status != o.Unavailable {
							t.Fatal("guard accepted/hidden", row)
						}
						row["result"] = "precise-unavailable-not-placement-pass"
						row["reason"] = o.Unavailable
						records = append(records, row)
						continue
					}
					before := len(c.Editor.Document.Placements)
					found := false
					reason := ""
					for cy := 0; cy < 64 && !found; cy++ {
						for cx := 0; cx < 64 && !found; cx++ {
							x, y := editorOverviewScreen(cx*24+12, cy*18+9)
							if e = c.handleEditor(Actions{Inside: true, X: x, Y: y, PointerMoved: true}); e != nil {
								t.Fatal(e)
							}
							if c.Editor.Shadow == nil {
								reason = c.Editor.ShadowError
								continue
							}
							if e = c.handleEditor(Actions{Inside: true, X: x, Y: y, PointerPressed: true, Selector1: true}); e != nil {
								t.Fatal(e)
							}
							if len(c.Editor.Document.Placements) > before {
								found = true
							} else {
								reason = c.Editor.Status
							}
						}
					}
					if !found {
						t.Fatal("supported menu choice failed actual placement consumer", row, reason)
					}
					p := c.Editor.Document.Placements[len(c.Editor.Document.Placements)-1]
					if p.Formation != o.Formation || p.Component != o.Component || p.Template != o.Template {
						t.Fatal("UI descriptor switched", row, p)
					}
					rows, e := gamestate.AuthoredComponents(data, c.Editor.Document, p)
					if e != nil || len(rows) != o.Cost {
						t.Fatal("source footprint changed", row, rows, e)
					}
					placed++
					if editorMapKey(w.WorldName) == "PHIL3" {
						phil3Placements++
					}
					row["result"] = "actual-selector-consumer-placed"
					row["components"] = rows
					row["x"], row["y"] = p.X, p.Y
					records = append(records, row)
				}
			}
		}
	}
	// R23 admits the 55 ingress entries previously counted as guards. Keep the
	// same supported-installation denominator and source list limit; do not
	// require a deliberately unsupported choice merely to satisfy this test.
	// Historical v3 rejection remains independently exercised by the P5 tests.
	// The P7 source catalogue lists 95 PHIL3 choices excluded only by map readiness.
	if phil3Placements != expectedPhil3 || placed-phil3Placements != 1883 || guarded != 0 || limited != 39 {
		t.Fatal("expected PHIL3 count, actual, total placements, guarded, capped", expectedPhil3, phil3Placements, placed, guarded, limited)
	}
	t.Logf("actual ready-map catalogue: supported placed%d; unavailable%d; total UI-exposed%d; source-list-limit excluded%d; descriptor candidates%d; map exclusions%d. Guards are not supported placement passes", placed, guarded, placed+guarded, limited, placed+guarded+limited, len(records)-placed-guarded-limited)
	if dir := os.Getenv("GS2000_P7_EDITOR_MATRIX_DIR"); dir != "" {
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		b, _ := json.MarshalIndent(records, "", " ")
		if e := os.WriteFile(filepath.Join(dir, "actual-editor-consumers.json"), append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestDeleteConfirmationSourceLayoutAndCancel(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	//Preparation31 owns these independently checked source literals. Older
	//prepared bundles are not silently rewritten by this rendering unit witness.
	if c.editorAssets().Text["delete_title"] == "" {
		t.Skip("Preparation31 Delete confirmation capability required")
	}
	for slot, label := range c.editorAssets().DeleteLabels {
		e.Stage = "control_delete_confirm"
		e.ControlChoice = slot
		before := gamestate.AuthoredRevision(e.Document)
		f := make([]byte, 64000)
		c.drawEditorControlOverlay(f)
		if f[59*320+79] != 240 || f[60*320+80] != 8 || f[150*320+240] != 15 {
			t.Fatal("source shell", slot)
		}
		expected := make([]byte, 64000)
		font := c.editorAssets().Fonts[3]
		width := font.Measure([]byte(label))
		_ = font.DrawClipped(expected, []byte(label), 82+(160-width)/2, 104, 10, 0, 0, [4]int{82, 60, 160, 89})
		for y := 104; y < 112; y++ {
			for x := 82; x < 242; x++ {
				if expected[y*320+x] != 0 && f[y*320+x] != expected[y*320+x] {
					t.Fatal("source selected option span", slot, x, y)
				}
			}
		}
		if _, err := c.handleEditorControlPopup(Actions{Cancel: true}, false, false, false); err != nil || e.Stage != "edit" || gamestate.AuthoredRevision(e.Document) != before {
			t.Fatal(slot, err, "cancel mutated")
		}
	}
	for _, text := range []string{"n", "N"} {
		e.Stage = "control_delete_confirm"
		before := gamestate.AuthoredRevision(e.Document)
		c.handleEditorControlPopup(Actions{Text: []rune(text)}, false, false, false)
		if e.Stage != "edit" || gamestate.AuthoredRevision(e.Document) != before {
			t.Fatal("NO input", text)
		}
	}
}

func TestFullDiagnosticReaderPreservesDraftAndCompleteReason(t *testing.T) {
	c := controlsEditor(t)
	reason := strings.Repeat("Source placement rejected: exact component and terrain ownership. ", 30)
	c.Editor.Status = reason
	lines := c.editorDiagnosticLines(reason)
	if strings.Join(lines, "") != reason || c.EditorDiagnosticPages(reason) < 2 {
		t.Fatal("complete reason lost")
	}
	before, _ := json.Marshal(c.Editor.Document)
	if e := c.handleEditor(Actions{Inside: true, X: 12, Y: 151, PointerPressed: true, Selector1: true}); e != nil || !c.Editor.Diagnostics || c.Editor.DiagnosticText != reason {
		t.Fatal("real status reader activation", e)
	}
	if e := c.handleEditor(Actions{Right: true}); e != nil || c.Editor.DiagnosticsPage != 1 {
		t.Fatal("reader page", e, c.Editor.DiagnosticsPage)
	}
	_ = c.editorFrame()
	if e := c.handleEditor(Actions{Enter: true}); e != nil || c.Editor.Diagnostics {
		t.Fatal("reader return", e)
	}
	after, _ := json.Marshal(c.Editor.Document)
	if string(before) != string(after) {
		t.Fatal("diagnostic changed authoring/RNG")
	}
}
