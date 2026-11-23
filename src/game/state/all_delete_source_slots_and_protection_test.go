package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"testing"
)

func controlFixture() (*assetdata.MissionGenerationData, *AuthoredDocument) {
	w := assetdata.MissionWorld{WorldName: "EURO0", Theatre: 0, THHD: make([]byte, 54), SNHD: make([]byte, 11), LAND: make([]byte, 64), Grid: make([]byte, 4096)}
	for i := range w.LAND {
		w.LAND[i] = 1
	}
	t := assetdata.MissionTables{EditorObjectTypes: make([]byte, 230*27), EditorModelCosts: make([]uint16, 230), EditorTheatreOffsets: make([]int8, 4), EditorWorldBaseCosts: make([]uint16, 14)}
	for fi := 0; fi < 13; fi++ {
		f := make([]byte, 39)
		copy(f, fmt.Sprintf("Formation%d", fi))
		binary.LittleEndian.PutUint16(f[25:], uint16(fi+1))
		flags := uint32(0x12000)
		if fi == 0 {
			flags = 0
		}
		binary.LittleEndian.PutUint32(f[35:], flags)
		w.FMHD = append(w.FMHD, f...)
		row := make([]byte, 8)
		row[0] = byte(fi + 1)
		row[1] = byte(fi)
		row[2] = 2
		if fi == 11 {
			row[2] = 1
		}
		row[3] = byte(fi % 2)
		w.FRMD = append(w.FRMD, row...)
		o := make([]byte, 32)
		o[10] = 3
		o[12] = byte(fi)
		w.OBJL = append(w.OBJL, o...)
		t.EditorObjectTypes[fi*27+1] = byte(fi % 10)
		copy(t.EditorObjectTypes[fi*27+2:], fmt.Sprintf("Object%d", fi))
	}
	data := &assetdata.MissionGenerationData{Version: 1, Worlds: []assetdata.MissionWorld{w}, Tables: t}
	d := &AuthoredDocument{Format: AuthoredFormat, Schema: AuthoredSchema, ID: fmt.Sprintf("%032x", 1), Compiler: AuthoredCompiler, Title: "Controls", World: w.WorldName, Dependency: authoredDependency(&w), Seed: 1}
	add := func(fi int, role string, builtin bool, kind int) {
		d.Placements = append(d.Placements, AuthoredPlacement{ID: fmt.Sprintf("%032x", len(d.Placements)+2), Role: role, Formation: fi, Component: fi, Template: fi, Kind: kind, X: 24*(fi+5) + 12, Y: 18*(fi+5) + 9, Builtin: builtin, SelectionProfile: AuthoredSelectionProfile})
	}
	for fi := 1; fi <= 11; fi++ {
		add(fi, "defender", false, 0)
	}
	add(12, "defender", true, 0)
	add(1, "base", false, 0)
	add(2, "primary", false, 7)
	add(3, "secondary", false, 3)
	d.Placements[0].Filled = true
	return data, d
}

func TestAllDeleteSourceSlotsAndProtection(t *testing.T) {
	// Independently transcribed from1da4:0eb0 IDIV/interleaved slot arithmetic.
	if got := []int{AuthoredDeleteCategory(8), AuthoredDeleteCategory(9), AuthoredDeleteCategory(10), AuthoredDeleteCategory(11), AuthoredDeleteCategory(12), AuthoredDeleteCategory(13), AuthoredDeleteCategory(14), AuthoredDeleteCategory(15), AuthoredDeleteCategory(16), AuthoredDeleteCategory(17)}; !reflect.DeepEqual(got, []int{4, 9, 3, 8, 2, 7, 1, 6, 0, 5}) {
		t.Fatal(got)
	}
	for slot := 0; slot < 19; slot++ {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			data, d := controlFixture()
			//Source last-N calls18C8 on the actual last descriptor. Put the
			//protected built-in first for these positive witnesses; the separate
			//P6 boundary test verifies a protected last row is not skipped.
			if slot >= 1 && slot <= 3 {
				d.Placements[0], d.Placements[11] = d.Placements[11], d.Placements[0]
			}
			before, _ := CloneAuthored(d)
			if slot == 0 {
				for len(d.Placements) < 75 {
					p := d.Placements[0]
					p.ID = fmt.Sprintf("%032x", 1000+len(d.Placements))
					d.Placements = append(d.Placements, p)
				}
			}
			if slot == 1 {
				d.Editor = &AuthoredEditorState{Profile: AuthoredControlProfile, Stream: 1, FillBoundary: 10}
			}
			if slot == 6 {
				d.Placements[10].SelectionProfile = AuthoredSelectionProfile
			}
			n, err := AuthoredBulkDelete(data, d, slot)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				t.Fatal("source slot failed to remove its positive witness")
			}
			if slot == 2 && n != 5 || slot == 3 && n != 1 || slot == 1 && n != 2 {
				t.Fatalf("wrong exact count %d", n)
			}
			for _, p := range before.Placements {
				if p.Role == "defender" && !p.Builtin {
					continue
				}
				found := false
				for _, q := range d.Placements {
					found = found || reflect.DeepEqual(p, q)
				}
				if !found {
					t.Fatalf("protected %s removed", p.Role)
				}
			}
			// Every bulk choice rejects retained layouts without mutation.
			q, _ := CloneAuthored(before)
			q.Initial = &AuthoredInitialCarrier{}
			raw, _ := json.Marshal(q)
			if _, err = AuthoredBulkDelete(data, q, slot); err == nil {
				t.Fatal("retained geometry accepted")
			}
			after, _ := json.Marshal(q)
			if string(raw) != string(after) {
				t.Fatal("failed delete mutated draft")
			}
		})
	}
	data, d := controlFixture()
	goalID := d.Placements[13].ID
	d.Placements[13] = d.Placements[0]
	d.Placements[13].ID, d.Placements[13].Role, d.Placements[13].Kind, d.Placements[13].Reference = goalID, "primary", 4, d.Placements[0].ID
	n, e := AuthoredBulkDelete(data, d, 18)
	//P7 raw1B35 proves the linked provenance carrier is no longer a
	//source ordinary row. Ten deletable rows plus the protected built-in remain.
	if e != nil || n != 10 {
		t.Fatal(n, e)
	}
	if d.Placements[0].Role != "defender" {
		t.Fatal("source built-in removed")
	}
}

func TestSelectionSourceTypesCostsAndLegacyExpansion(t *testing.T) {
	data, d := controlFixture()
	singles, limited, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if err != nil || limited || len(singles) != 12 {
		t.Fatal(len(singles), limited, err)
	}
	forms, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, true)
	if err != nil || len(forms) != 1 || forms[0].Component != 11 {
		t.Fatal(forms, err)
	}
	for _, o := range singles {
		if data.Worlds[0].FRMD[o.Component*8+2] != 2 {
			t.Fatal("single list contains formation")
		}
	}
	// A unique new model at exact9d42 is unavailable; used models remain allowed.
	data.Tables.EditorModelCosts[10] = 0x9d42
	d.Placements = nil
	_, limited, err = AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if err != nil || !limited {
		t.Fatal("memory limit not presented", err)
	}
	// Legacy descriptors continue their saved historical expansion; explicit
	// source profile stops before an alternate type2 variant, source2bae.
	w := &data.Worlds[0]
	w.FRMD = append(w.FRMD, []byte{255, 11, 2, 0, 0, 0, 0, 0}...)
	p := AuthoredPlacement{Formation: 12, Component: 12, Template: 12}
	w.FRMD[12*8+2] = 1
	old, _ := authoredPlacementComponents(w, p)
	p.SelectionProfile = AuthoredSelectionProfile
	now, _ := authoredPlacementComponents(w, p)
	if len(old) != 2 || len(now) != 1 {
		t.Fatal(old, now)
	}
}

func TestAllFillChoicesAtomicAndStoredStream(t *testing.T) {
	for slot := 0; slot < 6; slot++ {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			data, base := controlFixture()
			for i := range base.Placements {
				p := &base.Placements[i]
				switch p.Role {
				case "base":
					p.X, p.Y = 132, 1053
				case "primary":
					p.X, p.Y = 780, 567
				case "secondary":
					p.X, p.Y = 1092, 333
				}
			}
			d, _ := CloneAuthored(base)
			prefix := append([]AuthoredPlacement(nil), d.Placements...)
			seed := d.Seed
			n, err := AuthoredInteractiveFill(data, d, slot)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 || d.Editor == nil || d.Editor.Stream == seed {
				t.Fatal("Fill did not own persisted stream", n, d.Editor)
			}
			if !reflect.DeepEqual(prefix, d.Placements[:len(prefix)]) {
				t.Fatal("explicit placements changed")
			}
			if d.Seed != seed {
				t.Fatal("compilation seed rewritten")
			}
			remaining, err := AuthoredBudget(data, d)
			if err != nil || remaining != 0 {
				t.Fatal(remaining, err)
			}
			for _, p := range d.Placements[len(prefix):] {
				if !p.Filled || p.Role != "defender" {
					t.Fatal("missing Fill provenance")
				}
			}
			// A manual placement made after Fill belongs to the same sourceE290
			// suffix boundary, even though its explicit provenance is not Filled.
			manual := d.Placements[len(d.Placements)-1]
			manual.ID = fmt.Sprintf("%032x", 99999)
			manual.Filled = false
			d.Placements = append(d.Placements, manual)
			if removed, err := AuthoredBulkDelete(data, d, 1); err != nil || removed != n+1 {
				t.Fatal("source Fill boundary", removed, n, err)
			}
			q, _ := CloneAuthored(base)
			q.Placements = nil
			before := AuthoredRevision(q)
			if _, err = AuthoredInteractiveFill(data, q, slot); err == nil || AuthoredRevision(q) != before {
				t.Fatal("missing role accepted/mutated")
			}
		})
	}
}

func TestOriginalTablesNeutralCompileAndTacticalActorGuard(t *testing.T) {
	data := authoredTestData(t)
	base, in := authoredReady(t, data)
	for slot := 0; slot < 6; slot++ {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			d, _ := CloneAuthored(base)
			//P6 transcribes025C's rejection radius and admits the already
			//implemented actual component controller instead of a static-only
			//slot guard. Unknown source aliases still reject atomically inP3 tests.
			_, err := AuthoredInteractiveFill(data, d, slot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CompileAuthoredMission(data, d, in); err != nil {
				t.Fatal(err)
			}
			compiled := authoredDigest(d.Compiled)
			if _, err = CompileAuthoredMission(data, d, in); err != nil || authoredDigest(d.Compiled) != compiled {
				t.Fatal("unchanged BEGIN refilled", err)
			}
		})
	}
}

func TestOptionalControlFieldsKeepHistoricalRevision(t *testing.T) {
	_, d := controlFixture()
	for i := range d.Placements {
		d.Placements[i].SelectionProfile = ""
		d.Placements[i].Filled = false
	}
	raw, _ := json.Marshal(d)
	var legacy map[string]any
	_ = json.Unmarshal(raw, &legacy)
	if _, ok := legacy["editor"]; ok {
		t.Fatal("absent control carrier serialized")
	}
	for _, v := range legacy["placements"].([]any) {
		p := v.(map[string]any)
		if _, ok := p["selection_profile"]; ok {
			t.Fatal("absent selection profile serialized")
		}
		if _, ok := p["filled"]; ok {
			t.Fatal("absent Fill provenance serialized")
		}
	}
	q, _ := CloneAuthored(d)
	if AuthoredRevision(d) != AuthoredRevision(q) || authoredControlSeed(q) != d.Seed {
		t.Fatal("historical interpretation changed")
	}
}
