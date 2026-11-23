package state

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeepStrikeEdgeEntryHasNoFabricatedBase(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	options, err := AuthoredOptions(data, d, "base", 0)
	if err != nil {
		t.Fatal(err)
	}
	var chosen *AuthoredOption
	for i := range options {
		if options[i].Name == "Deep Strike" {
			chosen = &options[i]
			break
		}
	}
	if chosen == nil || chosen.Cost != 0 {
		t.Fatal("source entry choice/cost", options)
	}
	beforeBudget, err := AuthoredBudget(data, d)
	if err != nil {
		t.Fatal(err)
	}
	if err = PlaceAuthored(data, d, "base", 0, chosen.Formation, 40, 400); err != nil {
		t.Fatal(err)
	}
	var entry AuthoredPlacement
	for _, p := range d.Placements {
		if p.Role == "base" {
			entry = p
		}
	}
	if entry.Entry == nil || entry.Entry.Profile != AuthoredEntryProfile || entry.X != 60 || entry.Y != 405 {
		t.Fatal("source strict edge search", entry)
	}
	if err = ValidateAuthored(data, d); err != nil {
		t.Fatal(err)
	}
	remaining, err := AuthoredBudget(data, d)
	if err != nil || remaining <= beforeBudget {
		t.Fatal("entry charged physical base components", remaining, beforeBudget, err)
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	point := MissionPoint{20459, 340012}
	base := m.Objectives["base"]
	if len(m.Objects) != 80 || m.Objects[0].Point() != point || base.Point != point || base.ObjectIndex != 0 || base.RawKind != 0 || base.DisplayKind != 0 || base.Count != 0 || m.Conditions.Flags&0x800 == 0 || d.Compiled.Entry == nil {
		t.Fatal("entry/start/corridor actual owners", base, m.Objects[0], m.Conditions)
	}
	for _, off := range []int{34, 42} {
		if int32(binary.LittleEndian.Uint32(m.Targets.Records[0][off:])) != point.X || int32(binary.LittleEndian.Uint32(m.Targets.Records[0][off+4:])) != point.Y {
			t.Fatal("inactive TARGET entry/return anchor", off)
		}
	}
	if binary.LittleEndian.Uint16(m.Targets.Records[0][32:]) != 0 {
		t.Fatal("invented physical base goal")
	}
	for _, o := range m.Objects[9:] {
		if o.Formation == entry.Formation {
			t.Fatal("entry expanded into fabricated base OBJECT", o)
		}
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
	if _, err = store.Save(d, ""); err != nil {
		t.Fatal(err)
	}
	reopened, _, err := store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := cloneAuthoredRealization(reopened.Compiled)
	again, err := CompileAuthoredMission(data, reopened, in)
	if err != nil || !reflect.DeepEqual(frozen, reopened.Compiled) || again.Conditions.Flags&0x800 == 0 {
		t.Fatal("reopen/BEGIN changed entry", err)
	}
	exported, err := NewAuthoredFromMission(data, m, "Entry exported", 44)
	if err != nil {
		t.Fatal("entry Export", err)
	}
	if exported.Initial == nil || exported.Initial.Profile != "native-clean-initial-layout-v2" || exported.Initial.Layout.Entry == nil {
		t.Fatal("entry Export dropped frozen source owner")
	}
	exportedMission, err := CompileAuthoredMission(data, exported, in)
	if err != nil || exportedMission.ObjectHash != m.ObjectHash || !reflect.DeepEqual(exportedMission.Targets, m.Targets) || exportedMission.Conditions.Flags&0x800 == 0 {
		t.Fatal("entry Export BEGIN changed realized geometry", err)
	}
	exported.Initial.Layout.Entry.X++
	if ValidateAuthored(data, exported) == nil {
		t.Fatal("modified retained entry accepted")
	}
	t.Logf("source directionformation%d entry%02d/%02d no base OBJECT TARGETformation%d anchor%v", entry.Formation, entry.X/96, 16+entry.Y/-72, base.Formation, base.Point)
}
