package state

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

// Operands independently transcribed from GS20f4:70d2 and1da4:1274,
// retained in Step47's byte-checked source windows and gulf-table-checks.
func TestEditorPrivateInitializationAndSourceMenu(t *testing.T) {
	data := authoredTestData(t)
	w, e := AuthoredWorld(data, "GULF5.WRL")
	if e != nil {
		t.Fatal(e)
	}
	fr, fh := bytes.Clone(w.FRMD), bytes.Clone(w.FMHD)
	d, e := NewAuthoredDocument(data, w.WorldName, "P3 source", 1)
	if e != nil {
		t.Fatal(e)
	}
	options, e := AuthoredOptions(data, d, "base", 0)
	if e != nil {
		t.Fatal(e)
	}
	ids, names := []int{}, []string{}
	for _, o := range options {
		ids = append(ids, o.Formation)
		names = append(names, o.Name)
	}
	if !reflect.DeepEqual(ids, []int{38, 42, 32, 46, 47, 48}) || !reflect.DeepEqual(names, []string{"Base", "Base #2", "Deep Strike", "Sea Base", "Sea Base", "Sea Base"}) {
		t.Fatal(ids, names)
	}
	if options[0].Fixed || !options[3].Fixed {
		t.Fatal("movable Base/fixed Sea Base", options)
	}
	old := AuthoredRevision(d)
	// P5 closes the valid source619C constructor. Retain an independent
	// outside-domain rejection here; never clamp an invalid cursor into entry.
	if e = PlaceAuthored(data, d, "base", 0, 32, -20, 500); e == nil || !strings.Contains(e.Error(), "cursor outside source map") {
		t.Fatal(e)
	}
	now := AuthoredRevision(d)
	if old != now {
		t.Fatal("failed Deep Strike changed draft")
	}
	singles, limited, e := AuthoredEditorOptions(data, d, "defender", 0, 10, false)
	if e != nil || limited {
		t.Fatal(e, limited)
	}
	got := [][4]int{}
	for _, o := range singles {
		got = append(got, [4]int{o.Formation, o.Component, int(w.FRMD[o.Component*8+1]), int(w.FRMD[o.Component*8+3])})
	}
	want := [][4]int{{166, 322, 22, 147}, {199, 538, 66, 179}, {213, 652, 75, 17}, {213, 653, 4, 17}, {213, 654, 44, 130}, {230, 707, 24, 0}, {230, 708, 25, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("source singleton order", got)
	}
	ew, _ := AuthoredEditorWorld(data, w.WorldName)
	for _, ci := range []int{322, 538} {
		if w.FRMD[ci*8+2] != 1 || ew.FRMD[ci*8+2] != 2 {
			t.Fatal("private singleton", ci)
		}
	}
	if ew.FRMD[3*8+2] != w.FRMD[3*8+2] {
		t.Fatal("unlisted model changed")
	}
	if !bytes.Equal(fr, w.FRMD) || !bytes.Equal(fh, w.FMHD) {
		t.Fatal("canonical source mutated")
	}
	oldp := AuthoredPlacement{Formation: 166, Component: 322, Template: singles[0].Template, SelectionProfile: AuthoredSelectionProfileP2}
	np := oldp
	np.SelectionProfile = AuthoredSelectionProfile
	a, e := AuthoredComponents(data, d, oldp)
	if e != nil {
		t.Fatal(e)
	}
	b, e := AuthoredComponents(data, d, np)
	if e != nil {
		t.Fatal(e)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatal(a, b)
	}
	// Source private flag is derived only in the new view, not a durable edit.
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		ci, e := authoredFirst(ew, fi)
		if e == nil && ew.FRMD[ci*8+2] == 1 && binary.LittleEndian.Uint32(ew.FMHD[fi*39+35:])&0x80000 == 0 {
			t.Fatal("private formation marker", fi)
		}
	}
}
func TestEditorComponentTalliesAndHistoricalSelection(t *testing.T) {
	data := authoredTestData(t)
	d, _ := NewAuthoredDocument(data, "GULF5.WRL", "Tallies", 1)
	w, _ := AuthoredWorld(data, d.World)
	o, _ := AuthoredOptions(data, d, "base", 0)
	if e := PlaceAuthored(data, d, "base", 0, o[0].Formation, 700, 700); e != nil {
		t.Fatal(e)
	}
	counts, e := AuthoredDefenderCounts(data, d)
	if e != nil {
		t.Fatal(e)
	}
	want := [10]int{}
	for _, p := range d.Placements {
		rows, e := authoredPlacementComponents(w, p)
		if e != nil {
			t.Fatal(e)
		}
		for _, ci := range rows {
			want[min(int(data.Tables.EditorObjectTypes[int(w.FRMD[ci*8+1])*27+1]), 9)]++
		}
	}
	if counts != want {
		t.Fatal(counts, want)
	}
	if n := func() int {
		s := 0
		for _, v := range counts {
			s += v
		}
		return s
	}(); n <= len(d.Placements) {
		t.Fatal("counts must include multiple physical components", counts)
	}
}

func TestEditorActiveDefenderCategoryGuard(t *testing.T) {
	data := authoredTestData(t)
	d, e := NewAuthoredDocument(data, "GULF5.WRL", "Alias boundary", 1)
	if e != nil {
		t.Fatal(e)
	}
	w, _ := AuthoredWorld(data, d.World)
	// Independent source operands: Pickup151/228 have OBJECT kind15h and
	// editor rawcategory10 (UI11), outside the ten ordinarydefender controls.
	opts, _, e := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if e != nil {
		t.Fatal(e)
	}
	seen := map[byte]bool{}
	for _, o := range opts {
		model := w.FRMD[o.Component*8+1]
		if model != 151 && model != 228 {
			continue
		}
		seen[model] = true
		if w.OBJL[o.Template*32+10] != 0x15 || data.Tables.EditorObjectTypes[int(model)*27+1] != 10 {
			t.Fatal("source alias operands", model)
		}
		before := AuthoredRevision(d)
		if e = PlaceAuthoredOption(data, d, "defender", 0, o, 400, 400); e == nil || !strings.Contains(e.Error(), "category11") {
			t.Fatal(model, e)
		}
		if before != AuthoredRevision(d) {
			t.Fatal("rejected alias changed draft/stream")
		}
		p := AuthoredPlacement{ID: "f5dc45c1b2f403110000000000000151", Role: "defender", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: AuthoredSelectionProfile, X: 396, Y: 405, Terrain: w.Grid[16+64*22] & 63}
		q, _ := CloneAuthored(d)
		q.Placements = append(q.Placements, p)
		if e = ValidateAuthored(data, q); e == nil || !strings.Contains(e.Error(), "category11") {
			t.Fatal("active document alias admitted", model, e)
		}
		// The new check cannot relabel historical interpretations or actual goal
		// owners as ordinary defenders. This isolates the admission change.
		p.SelectionProfile = AuthoredSelectionProfileP2
		if e = authoredDefenderAdmission(data, w, p); e != nil {
			t.Fatal("historicalv1 altered", e)
		}
		p.SelectionProfile = ""
		if e = authoredDefenderAdmission(data, w, p); e != nil {
			t.Fatal("historical raw altered", e)
		}
		p.SelectionProfile = AuthoredSelectionProfile
		p.Role = "primary"
		if e = authoredDefenderAdmission(data, w, p); e != nil {
			t.Fatal("goal owner altered", e)
		}
	}
	if !seen[151] || !seen[228] {
		t.Fatal("both source aliases required", seen)
	}
	legal, _, e := AuthoredEditorOptions(data, d, "defender", 0, 10, false)
	if e != nil || len(legal) != 7 {
		t.Fatal(e, len(legal))
	}
	if e = PlaceAuthoredOption(data, d, "defender", 0, legal[0], 400, 400); e != nil {
		t.Fatal("legal sourceBunker rejected", e)
	}
	// Inject only a classified source operand in a synthetic Fillfixture. A
	// failed alias must retain its detached draft and stored stream exactly.
	for _, interactive := range []bool{true, false} {
		fd, q := controlFixture()
		fd.Tables.EditorObjectTypes[1] = 10
		before := AuthoredRevision(q)
		if interactive {
			_, e = AuthoredInteractiveFill(fd, q, 0)
		} else {
			g, err := authoredPopulation(&fd.Worlds[0], authoredRNG(1))
			if err != nil {
				t.Fatal(err)
			}
			_, e = authoredFill(fd, g, q.Placements, q.Placements[len(q.Placements)-3])
		}
		if e == nil || !strings.Contains(e.Error(), "category11") {
			t.Fatal("aliased sourceFill was not rejected", interactive, e)
		}
		if AuthoredRevision(q) != before {
			t.Fatal("failed sourceFill changed draft/stream", interactive)
		}
	}
}
