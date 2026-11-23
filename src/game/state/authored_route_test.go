package state

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAuthoredInterceptRouteHeaderChildrenAndSavedRealization(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		t.Fatal(err)
	}
	var option AuthoredOption
	found := false
	for _, o := range opts {
		flags := binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])
		if flags&0x180 != 0 {
			option = o
			found = true
			break
		}
	}
	if !found {
		t.Fatal("source ground moving formation missing")
	}
	if err = PlaceAuthoredOption(data, d, "defender", 0, option, 400, 400); err != nil {
		t.Fatal(err)
	}
	unit := d.Placements[len(d.Placements)-1]
	if err = ValidateAuthored(data, d); err != nil {
		t.Fatal("ordinary absolute source group rejected", err)
	}
	ordinary, _ := CloneAuthored(d)
	ordinaryMission, err := CompileAuthoredMission(data, ordinary, in)
	if err != nil || len(ordinaryMission.Paths) != 2 {
		t.Fatal("ordinary source group invented route", err)
	}
	for _, o := range ordinaryMission.Objects[9:] {
		if o.Formation == unit.Formation && o.Raw[0]&3 != 0 {
			t.Fatal("ordinary group made relative", o)
		}
	}
	if err = PlaceAuthored(data, d, "primary", 2, unit.Formation, unit.X, unit.Y); err != nil {
		t.Fatal(err)
	}
	if ValidateAuthored(data, d) == nil {
		t.Fatal("incomplete second destination admitted")
	}
	reached := false
	for _, point := range [][2]int{{600, 400}, {400, 600}, {500, 500}, {unit.X, unit.Y}, {800, 700}} {
		if err = SetAuthoredRoutePoint(data, d, "primary", point[0], point[1]); err == nil {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatal("source planner found no admitted destination", err)
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	goal := m.Objectives["primary"]
	header := m.Objects[goal.ObjectIndex]
	if len(m.Objects) != 80 || len(m.Paths) <= 2 || goal.RawKind != 3 || goal.DisplayKind != 3 || header.Raw[10] != 0x40 || header.Point() != (MissionPoint{0, 2}) {
		t.Fatal("actual compiler header/path/goal", goal, header, len(m.Paths))
	}
	rows, err := AuthoredComponents(data, d, unit)
	if err != nil {
		t.Fatal(err)
	}
	if goal.Count != len(rows)-len(rows)/2 {
		t.Fatal("dummy added to destruction requirement", goal.Count, len(rows))
	}
	for i, ci := range rows {
		row := m.Objects[goal.ObjectIndex+1+i]
		raw := w.FRMD[ci*8 : ci*8+8]
		if row.Raw[0]&1 == 0 || row.Raw[0]&0x10 == 0 || row.Point() != (MissionPoint{int32(int16(binary.LittleEndian.Uint16(raw[4:]))), int32(int16(binary.LittleEndian.Uint16(raw[6:])))}) {
			t.Fatal("actual source relative child", i, row)
		}
	}
	if goal.Point != m.Paths[2].Point || goal.OriginalPoint != goal.Point || goal.Endpoint == nil || *goal.Endpoint != m.Paths[len(m.Paths)-1].Point {
		t.Fatal("TARGET path endpoints", goal, m.Paths)
	}
	r := m.Targets.Records[2]
	if int32(binary.LittleEndian.Uint32(r[54:])) != goal.Endpoint.X || int32(binary.LittleEndian.Uint32(r[58:])) != goal.Endpoint.Y {
		t.Fatal("TARGET54/58 endpoint not carried")
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
	again, err := CompileAuthoredMission(data, loaded, in)
	if err != nil || !reflect.DeepEqual(loaded.Compiled, frozen) || !reflect.DeepEqual(again.Paths, m.Paths) {
		t.Fatal("saved BEGIN recompiled/changed route", err)
	}
	exported, err := NewAuthoredFromMission(data, m, "Route exported", 44)
	if err != nil {
		t.Fatal("source route Export", err)
	}
	if exported.Initial.Profile != "native-clean-initial-layout-v2" {
		t.Fatal("new route used historical count interpretation")
	}
	exportedMission, err := CompileAuthoredMission(data, exported, in)
	if err != nil || exportedMission.ObjectHash != m.ObjectHash || !reflect.DeepEqual(exportedMission.Paths, m.Paths) {
		t.Fatal("route Export changed launch", err)
	}
	t.Logf("source formation%d/component%d header%d group2 children%d PATH%d actual first%v last%v", unit.Formation, unit.Component, goal.ObjectIndex, len(rows), len(m.Paths), goal.Point, *goal.Endpoint)
}
