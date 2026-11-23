package state

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestNearestExistingGoalReverseAndDifferentFormation(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "Nearest", 1)
	if err != nil {
		t.Fatal(err)
	}
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if err != nil || len(opts) < 2 {
		t.Fatal(err, len(opts))
	}
	var a, b AuthoredOption
	a = opts[0]
	for _, o := range opts[1:] {
		if o.Formation != a.Formation {
			b = o
			break
		}
	}
	if b.Name == "" {
		t.Fatal("no distinct source formations")
	}
	makep := func(o AuthoredOption, id string, x int) AuthoredPlacement {
		return AuthoredPlacement{ID: id, Role: "defender", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile, X: x, Y: 400}
	}
	d.Placements = []AuthoredPlacement{makep(a, "00000000000000000000000000000001", 400), makep(b, "00000000000000000000000000000002", 500)}
	p, err := NearestAuthoredGoal(data, d, 401, 400)
	if err != nil || p.ID != d.Placements[0].ID {
		t.Fatal("earlier different Formation not selected", p, err)
	}
	p, err = NearestAuthoredGoal(data, d, 450, 400)
	if err != nil || p.ID != d.Placements[1].ID {
		t.Fatal("newest tie", p, err)
	}
	if err = PlaceAuthored(data, d, "primary", 4, b.Formation, 401, 400); err != nil {
		t.Fatal(err)
	}
	goal := d.Placements[len(d.Placements)-1]
	if goal.Reference != d.Placements[0].ID || goal.Formation != a.Formation || goal.Component != a.Component {
		t.Fatal("actual goal carrier differs", goal)
	}
}
func TestPreviewNoMutationDrawsOrWholeDocumentAllocation(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "Preview", 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := NewAuthoredEditorContext(data, d.World)
	if err != nil {
		t.Fatal(err)
	}
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	before, err := CloneAuthored(d)
	if err != nil {
		t.Fatal(err)
	}
	alloc := testing.AllocsPerRun(100, func() { _, _ = ctx.Preview(d, "defender", 0, opts[0], 400, 400) })
	if alloc > 2 {
		t.Fatalf("shadow allocates %.0f objects/update", alloc)
	}
	if !reflect.DeepEqual(before, d) {
		t.Fatal("preview mutated document")
	}
}
func TestSupportOptionsReservationAndSourceTemplates(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "Support", 1)
	if err != nil {
		t.Fatal(err)
	}
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 6 {
		t.Fatalf("source chooser count=%d: %+v", len(options), options)
	}
	start, _ := AuthoredBudget(data, d)
	w, _ := AuthoredWorld(data, d.World)
	artillery, air := 0, 0
	for _, o := range options[1:] {
		err = SetAuthoredSupport(data, d, o)
		if err != nil {
			t.Fatal(o.Name, err)
		}
		n, _ := AuthoredBudget(data, d)
		kind := w.OBJL[o.Template*32+10]
		if kind == 0x19 {
			artillery++
			if o.Cost != 3 || n != start-3 {
				t.Fatal("battery reservation", o, n, start)
			}
		} else if kind == 0x11 {
			air++
			if o.Cost != 1 || n != start-1 {
				t.Fatal("air reservation", o, n, start)
			}
		} else {
			t.Fatal("wrong sourcekind", kind)
		}
		if d.Support.Template != o.Template || d.Support.Component != o.Component {
			t.Fatal("identity lost")
		}
	}
	if artillery != 2 || air != 3 {
		t.Fatal(artillery, air)
	}
	if err = SetAuthoredSupport(data, d, options[0]); err != nil {
		t.Fatal(err)
	}
	n, _ := AuthoredBudget(data, d)
	if d.Support != nil || n != start {
		t.Fatal("None reservation leaked", n, start)
	}
	// A source-kind is not substituted for a raw WSHD prefix.
	for _, o := range options[1:] {
		if w.OBJL[o.Template*32+10] == 0x19 {
			if err = SetAuthoredSupport(data, d, o); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	s := d.Support
	s.Template++
	if authoredSupportValidate(data, d) == nil {
		t.Fatal("wrong template admitted")
	}
}
func TestArtilleryCompilerFullRowsAndFillReservation(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range options {
		if o.Cost == 3 {
			if err = SetAuthoredSupport(data, d, o); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if d.Support == nil {
		t.Fatal("battery absent")
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if m.Support == nil || len(m.Objects) != 80 || !reflect.DeepEqual(m.Support.Rows, []int{77, 78, 79}) {
		t.Fatal("source reservation/rows", m.Support, len(m.Objects))
	}
	if m.Support.PrefixRaw[0] < 25 || m.Support.PrefixRaw[0] > 27 || m.Support.PrefixRaw[1] != 0 {
		t.Fatal("raw weapon preamble", m.Support)
	}
	w, _ := AuthoredWorld(data, d.World)
	for j, row := range m.Support.Rows {
		ci := d.Support.Component + j
		ti, e := authoredTemplate(w, ci)
		if e != nil {
			t.Fatal(e)
		}
		want := append([]byte(nil), w.OBJL[ti*32:ti*32+32]...)
		got := m.Objects[row].Raw
		copy(want[2:10], got[2:10])
		want[11] = w.FRMD[d.Support.Component*8+3]
		if !reflect.DeepEqual(want, got[:]) {
			t.Fatalf("row%d not full template", row)
		}
		if got[10] != 0x19 || binary.LittleEndian.Uint16(got[27:]) != binary.LittleEndian.Uint16(w.OBJL[ti*32+27:]) {
			t.Fatal("kind/score preserved")
		}
	}
	before := AuthoredRevision(d)
	saved, _ := cloneAuthoredRealization(d.Compiled)
	if _, err = CompileAuthoredMission(data, d, in); err != nil {
		t.Fatal(err)
	}
	if before != AuthoredRevision(d) || !reflect.DeepEqual(saved, d.Compiled) {
		t.Fatal("unchanged BEGIN refilled")
	}
}

func TestSupportLaunchRejectsWrongCarrierBeforeInitialization(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range options {
		if o.Cost == 3 {
			if err = SetAuthoredSupport(data, d, o); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateSupportLaunch(m); err != nil {
		t.Fatal(err)
	}
	original := *m.Support
	for _, kind := range []string{"prefix", "duplicate", "component", "formation", "profile"} {
		cfg := original
		cfg.Rows = append([]int(nil), original.Rows...)
		m.Support = &cfg
		switch kind {
		case "prefix":
			cfg.PrefixRaw[0] = 4
		case "duplicate":
			cfg.Rows[1] = cfg.Rows[0]
		case "component":
			cfg.Selection.Component++
		case "formation":
			cfg.Selection.Formation++
		case "profile":
			cfg.Profile = "unknown"
		}
		if ValidateSupportLaunch(m) == nil {
			t.Fatal("admitted malformed Support", kind)
		}
	}
	m.Support = nil // strict historical mission interpretation adds no prefix.
	if err = ValidateSupportLaunch(m); err != nil {
		t.Fatal(err)
	}
}

func TestAirSupportRetainsCompiledDocumentButRejectsExecution(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range options {
		if o.Cost == 1 && o.Component >= 0 {
			if err = SetAuthoredSupport(data, d, o); err != nil {
				t.Fatal(err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("source air option absent")
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if m.Support == nil || m.Support.Mode != 2 || len(m.Support.Rows) != 1 || m.Support.Rows[0] != 79 || m.Objects[79].Raw[10] != 0x11 || m.Support.PrefixRaw[1] != 24 || m.Support.PrefixRaw[0] < 22 || m.Support.PrefixRaw[0] > 23 {
		t.Fatal("sourceair carrier", m.Support)
	}
	if ValidateSupportLaunch(m) == nil || AuthoredExecutionReady(data, d) == nil {
		t.Fatal("unfinishedaircontroller admitted")
	}
	store := AuthoredStore{Directory: t.TempDir()}
	rev, err := store.Save(d, "")
	if err != nil {
		t.Fatal("air descriptor SAVE", err)
	}
	loaded, readRev, err := store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if readRev != rev || !reflect.DeepEqual(loaded.Support, d.Support) || !reflect.DeepEqual(loaded.Compiled, d.Compiled) {
		t.Fatal("air descriptor/readback lost identity")
	}
}

func TestEditorOrdinaryDescriptor75BoundaryDoesNotCountSpecials(t *testing.T) {
	data := authoredTestData(t)
	d, _ := authoredReady(t, data)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var o AuthoredOption
	found := false
	for _, v := range opts {
		if !v.Fixed && v.Cost == 1 {
			o = v
			found = true
			break
		}
	}
	if !found {
		t.Fatal("movable source singleton absent")
	}
	candidate := 0
	for authoredOrdinaryDescriptorCount(d) < 75 {
		if candidate >= 4096 {
			t.Fatal("source fixture exhausted")
		}
		x, y := (candidate%64)*24+12, (candidate/64)*18+9
		candidate++
		if err = PlaceAuthoredOption(data, d, "defender", 0, o, x, y); err != nil {
			continue
		}
	}
	// Base and two objectives remain distinct fields, not C018 list entries.
	if len(d.Placements) != 78 {
		t.Fatal("source C018 ownership includes special controls", len(d.Placements))
	}
	before := AuthoredRevision(d)
	if err = PlaceAuthoredOption(data, d, "defender", 0, o, 1000, 1000); err == nil {
		t.Fatal("76th ordinary descriptor admitted")
	}
	if _, _, err = AuthoredEditorOptions(data, d, "defender", 0, 0, false); err == nil {
		t.Fatal("full C018 stilladmitsmenu")
	}
	if before != AuthoredRevision(d) {
		t.Fatal("full source list rejected aftermutation")
	}
	if _, err = AuthoredBudget(data, d); err == nil {
		t.Fatal("negative component budget should independently blockSAVE")
	}
}
