package menus

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Actual editor commands establish a legal short authored layout. This does not
// claim a continuously flown sortie: human pickup/return acceptance is separate.
func TestShortPickupUIHasLegalMeasuredSourceRange(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	bx, by := 0, 0
	for _, p := range doc.Placements {
		if p.Role == "base" {
			w, err := gamestate.AuthoredWorld(c.editorData(), doc.World)
			if err != nil || doc.World != "EURO6.WRL" || p.Formation != 7 || binary.LittleEndian.Uint16(w.FMHD[p.Formation*39+25:]) != 1 || binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x2000 == 0 {
				t.Fatal("current source Base identity", p.Formation, err)
			}
			bx, by = p.X, p.Y
		}
	}
	px, py := editorOverviewScreen(bx+48, by)
	sx, sy := editorOverviewScreen(bx, by)
	t.Logf("original base editor=%d,%d; pickup sourceUI=%d,%d drop sourceUI=%d,%d", bx, by, px, py, sx, sy)
	editorKey(t, c, "e")
	for _, q := range []struct {
		key, role  string
		kind, x, y int
	}{{"p", "primary", 7, px, py}, {"s", "secondary", 5, sx, sy}} {
		editorKey(t, c, q.key)
		r := c.Editor.popupArea(q.kind, len(c.editorAssets().GoalLabels))
		editorClick(t, c, r[0]+2, r[1]+2)
		if c.Editor.Stage != "option" {
			t.Fatal("goal kind was not accepted", q, c.Editor.Stage)
		}
		//E288zero does not warp the pointer into option0. An Enter over the
		//previous kind-menu coordinate is not a selection. Reach the actual
		//row with a physical press before attempting a map placement.
		r = c.Editor.popupArea(0, len(c.Editor.Options))
		editorClick(t, c, r[0]+2, r[1]+2)
		if !c.Editor.Held || c.Editor.Role != q.role || c.Editor.Kind != q.kind {
			t.Fatal("goal option was not accepted", q, c.Editor.Stage)
		}
		before := gamestate.AuthoredRevision(c.Editor.Document)
		editorAction(t, c, Actions{X: q.x, Y: q.y, Inside: true})
		if c.Editor.Shadow == nil {
			t.Fatal("source placement has no eligible shadow", q, c.Editor.ShadowError)
		}
		shadow := *c.Editor.Shadow
		if before != gamestate.AuthoredRevision(c.Editor.Document) {
			t.Fatal("hover changed authored state")
		}
		// Preview and the complete accepted-press placer are separate paths.
		// Check the cached shadow against a detached authoritative placement
		// before the real press; neither path may relax source site eligibility.
		detached, err := gamestate.CloneAuthored(c.Editor.Document)
		if err != nil {
			t.Fatal(err)
		}
		x, y := c.editorPoint()
		if err := gamestate.PlaceAuthoredOption(c.editorData(), detached, q.role, q.kind, c.Editor.Options[c.Editor.Option], x, y); err != nil {
			t.Fatal("shadow disagrees with authoritative source placement", q, err)
		}
		matched := false
		for _, p := range detached.Placements {
			matched = matched || p.Role == q.role && p.Kind == q.kind && p.Formation == shadow.Formation && p.Component == shadow.Component && p.Template == shadow.Template && p.X == shadow.X && p.Y == shadow.Y && p.Terrain == shadow.Terrain
		}
		if !matched {
			t.Fatal("cached shadow differs from authoritative placement", q, shadow)
		}
		editorClick(t, c, q.x, q.y)
		placed := false
		for _, p := range c.Editor.Document.Placements {
			if p.Role == q.role && p.Kind == q.kind && p.X == shadow.X && p.Y == shadow.Y {
				placed = true
			}
		}
		if !placed {
			t.Fatal("actual source placement not published", q, shadow)
		}
		// Source1da4:27b6..27bd completes a special command rather than
		// preserving its shadow as an ordinary repeatable placement.
		if c.Editor.Held || c.Editor.Shadow != nil || c.Editor.Role != "" {
			t.Fatal("accepted special placement retained its command owner", q)
		}
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil && c.Editor.Stage == "title" {
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenMissionFiles {
		t.Fatal(c.Screen, c.Editor)
	}
	d, _, e := c.Files.Store.Load(c.Files.Entries[c.Files.Selected].ID)
	if e != nil {
		t.Fatal(e)
	}
	if d.Compiled == nil {
		t.Fatal("SAVE didnotcompile")
	}
	b, p := d.Compiled.Objectives["base"].Point, d.Compiled.Objectives["primary"].Point
	// Independent source navigation2*max+min at SAR4 coordinates, not Euclidean metres.
	x, y := int(math.Abs(float64((p.X>>4)-(b.X>>4)))), int(math.Abs(float64((p.Y>>4)-(b.Y>>4))))
	metric := 2*max(x, y) + min(x, y)
	t.Logf("base=%+v pickup=%+v source navigation metric=%d display=%d.%dkm", b, p, metric, metric/1000, (metric%1000)/100)
	if metric < 1500 || metric > 2600 {
		t.Fatal("legal short layout is not near2km", metric)
	}
	if dir := os.Getenv("GS2000_P3_SHORT_PICKUP_DIR"); dir != "" {
		//Explicit research output only: retain the real UI-saved document for
		//a later continuous flight. No flight state, goal completion or personal
		//career is supplied. Exclusive publication preserves earlier witnesses.
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		saved := filepath.Join(c.Files.Store.Directory, d.ID+".g2m")
		raw, err := os.ReadFile(saved)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, d.ID+".g2m")
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(raw)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		h := sha256.Sum256(raw)
		record := map[string]any{"scope": "UI-authored and compiled only; continuous Blackhawk pickup/return and human acceptance pending", "document": out, "sha256": hex.EncodeToString(h[:]), "world": d.World, "seed": d.Seed, "base_formation": 7, "base_editor": [2]int{bx, by}, "edit_pickup_source_ui": [2]int{px, py}, "edit_drop_source_ui": [2]int{sx, sy}, "base": b, "pickup": p, "navigation_metric": metric, "display_km": float64(metric/100) / 10, "procedure": "pAuthorThroughUI SAVE; EDIT; PRIMARY Pick up Troops option0 at92/58; SECONDARY Drop off Troops option0 at87/58; SAVE. Accept an actual player Blackhawk and valid cargo loadout in ordinary preflight; no staged launch pose."}
		encoded, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "layout.json"), append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
