package state

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSearchDestroyUsesExistingUnitAndIndependentSearchMarker(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 10, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	if err = PlaceAuthoredOption(data, d, "defender", 0, opts[0], 400, 400); err != nil {
		t.Fatal(err)
	}
	unit := d.Placements[len(d.Placements)-1]
	if err = PlaceAuthored(data, d, "primary", 0, unit.Formation, unit.X, unit.Y); err != nil {
		t.Fatal(err)
	}
	if ValidateAuthored(data, d) == nil {
		t.Fatal("incomplete second marker admitted")
	}
	if err = SetAuthoredSearchPoint(data, d, "primary", 500, 650); err != nil {
		t.Fatal(err)
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	goal := m.Objectives["primary"]
	r := m.Targets.Records[2]
	if goal.RawKind != 1 || goal.DisplayKind != 3 {
		t.Fatal("Search & Destroy relabelled", goal)
	}
	if goal.Point == goal.OriginalPoint || goal.Point != (MissionPoint{170499, 228537}) {
		t.Fatal(goal)
	}
	if int32(binary.LittleEndian.Uint32(r[42:])) != 170499 || int32(binary.LittleEndian.Uint32(r[46:])) != 228537 {
		t.Fatal("search marker did not reach TARGET", r)
	}
	var p AuthoredPlacement
	for _, q := range d.Placements {
		if q.Role == "primary" {
			p = q
		}
	}
	if p.Reference != unit.ID || p.Component != unit.Component || p.X != unit.X || p.Y != unit.Y {
		t.Fatal("unit relocated or reference lost", p, unit)
	}
	if len(m.Paths) != 2 {
		t.Fatal("invented intercept path", m.Paths)
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
	if _, err = store.Save(d, ""); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := cloneAuthoredRealization(loaded.Compiled)
	launched, err := CompileAuthoredMission(data, loaded, in)
	if err != nil || !reflect.DeepEqual(frozen, loaded.Compiled) || !reflect.DeepEqual(launched.Targets, m.Targets) {
		t.Fatal("reopen/BEGIN changed search realization", err)
	}
	// Supported same-object reassignment invalidates the old second marker.
	if err = PlaceAuthored(data, loaded, "primary", 0, unit.Formation, unit.X, unit.Y); err != nil {
		t.Fatal(err)
	}
	if ValidateAuthored(data, loaded) == nil {
		t.Fatal("reassignment silently reused search marker")
	}
}

func TestSearchRescueKeepsSourcePickupCarrierAndIndependentMarker(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	options, err := AuthoredOptions(data, d, "primary", 1)
	if err != nil || len(options) == 0 {
		t.Fatal("source rescue choices", err)
	}
	var chosen bool
	for _, o := range options {
		q, _ := CloneAuthored(d)
		if err = PlaceAuthored(data, q, "primary", 1, o.Formation, 400, 400); err != nil {
			continue
		}
		if err = SetAuthoredSearchPoint(data, q, "primary", 500, 650); err != nil {
			t.Fatal(err)
		}
		m, err := CompileAuthoredMission(data, q, in)
		if err != nil {
			t.Fatal(err)
		}
		g := m.Objectives["primary"]
		if g.RawKind != 6 || g.DisplayKind != 2 || g.Count != 1 {
			t.Fatal("source rescue raw/dispatch/count", g)
		}
		if m.Objects[g.ObjectIndex].Raw[10] != 0x15 {
			t.Fatal("rescue became different actor", m.Objects[g.ObjectIndex])
		}
		if g.Point != (MissionPoint{170499, 228537}) || g.Point == g.OriginalPoint {
			t.Fatal("search marker lost", g)
		}
		chosen = true
		break
	}
	if !chosen {
		t.Fatal("no source rescue site was admitted")
	}
}
