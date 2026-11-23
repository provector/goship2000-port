package state

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOrdinaryBatteryOwnsActualRowsWithoutInventedPrefix(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	w, _ := AuthoredEditorWorld(data, d.World)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	//P7 raw3744 rejects formation flags00400002. Preserve this historical
	//no-prefix constructor test with an explicitly supplied retained source
	//descriptor; it is not a claim that the current original chooser exposes
	//an ordinary artillery battery. Existing documents must keep these rows.
	found := false
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		flags := binary.LittleEndian.Uint32(w.FMHD[fi*39+35:])
		if flags&0x02000000 == 0 {
			continue
		}
		ci, e := authoredFirst(w, fi)
		if e != nil {
			continue
		}
		ti, e := authoredTemplate(w, ci)
		if e != nil || w.OBJL[ti*32+10] != 0x19 {
			continue
		}
		if flags&0x00400002 == 0 {
			t.Fatal("retained source battery should be excluded by source chooser mask00400002")
		}
		for _, o := range opts {
			if o.Formation == fi {
				t.Fatal("source-ineligible ordinary battery exposed in current chooser")
			}
		}
		p, e := authoredPlacement(w, fi, "defender", 0)
		if e != nil {
			t.Fatal(e)
		}
		p.SelectionProfile = AuthoredSelectionProfileP5
		pop, e := newMissionPopulation(w, authoredRNG(d.Seed), nil)
		if e != nil {
			t.Fatal(e)
		}
		cell, ok := pop.spiral(600/24, 63-500/18, flags)
		if !ok {
			t.Fatal("retained source fixture site missing")
		}
		p.X, p.Y = cell[0]*24+12, (63-cell[1])*18+9
		p.Terrain = w.Grid[p.X/24+64*(p.Y/18)] & 63
		d.Compiler = AuthoredCompilerP5
		d.Placements = append(d.Placements, p)
		d.Compiled = nil
		found = true
		break
	}
	if !found {
		t.Fatal("retained source battery descriptor missing")
	}

	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if m.Support == nil || m.Support.Profile != SupportRowsInitializationProfile || m.Support.Mode != 0 || m.Support.PrefixRaw != [2]byte{} || d.Support != nil || m.Support.LastControllerKind() != 0x19 {
		t.Fatal("ordinary battery was inert or falsely relabelled as special Support", m.Support)
	}
	if err = ValidateSupportLaunchP5(m); err != nil {
		t.Fatal(err)
	}
	if ValidateSupportLaunch(m) == nil {
		t.Fatal("old interpretation admitted the new no-prefix row owner")
	}
	q := *m.Support
	q.Rows = append([]int(nil), q.Rows...)
	q.Rows = q.Rows[1:]
	m.Support = &q
	if ValidateSupportInitialization(m) == nil {
		t.Fatal("missing actual battery row accepted")
	}
}

func TestKnownPriorCompiledProcessMemoryCanSaveANewDocument(t *testing.T) {
	data := authoredTestData(t)
	prior, in := authoredReady(t, data)
	prior.Compiler = AuthoredCompilerP5
	if _, err := CompileAuthoredMission(data, prior, in); err != nil {
		t.Fatal(err)
	}
	d, _ := authoredReady(t, data)
	d.Seed = 2
	opts, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range opts {
		if o.Cost == 3 {
			if err = SetAuthoredSupport(data, d, o); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err = CancelAuthoredSupport(data, d, prior); err != nil {
		t.Fatal(err)
	}
	if d.SupportMemory.SourceProfile != AuthoredSupportProcessMemoryProfile {
		t.Fatal("cross-document predecessor was relabelled as same document")
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Objects[77:80], prior.Compiled.Objects[77:80]) || m.Support.Mode != 0 {
		t.Fatal("NEW invented a prior suffix or active prefix")
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
	if _, err = store.Save(d, ""); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CompileAuthoredMission(data, loaded, in); err != nil {
		t.Fatal("persisted process predecessor failed exact readback", err)
	}
}

func TestSupportCancelRetainsActualRowsAndLiteralReservationOrder(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	opts, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	battery, air := AuthoredOption{}, AuthoredOption{}
	for _, o := range opts {
		if o.Cost == 3 {
			battery = o
		}
		if o.Cost == 1 {
			air = o
		}
	}
	if battery.Component == 0 || air.Component == 0 {
		t.Fatal("source support alternatives missing")
	}
	if err = SetAuthoredSupport(data, d, battery); err != nil {
		t.Fatal(err)
	}
	first, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	prior, _ := CloneAuthored(d)
	costBefore, _ := AuthoredBudget(data, d)
	if err = CancelAuthoredSupport(data, d, prior); err != nil {
		t.Fatal(err)
	}
	if d.Support != nil || d.SupportMemory == nil || AuthoredSupportRows(d) != 3 {
		t.Fatal("cancel lost inactive reservation")
	}
	costAfter, _ := AuthoredBudget(data, d)
	if costAfter != costBefore {
		t.Fatal("cancel refunded old cost", costAfter, costBefore)
	}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if m.Support == nil || !m.Support.HasRetainedPrefixPolicy() || m.Support.Mode != 0 || m.Support.PrefixRaw != [2]byte{} || !reflect.DeepEqual(m.Objects[77:80], first.Objects[77:80]) {
		t.Fatal("cancel invented prefix/rows", m.Support)
	}
	if err = ValidateSupportInitialization(m); err != nil {
		t.Fatal(err)
	}
	if ValidateSupportLaunch(m) == nil {
		t.Fatal("historical interpretation admitted new retained-memory profile")
	}
	if err = SetAuthoredSupport(data, d, opts[0]); err != nil {
		t.Fatal(err)
	}
	if AuthoredSupportRows(d) != 3 || AuthoredSupportCaption(d) != nil {
		t.Fatal("None after cancellation released uncredited reservation or kept caption")
	}
	if err = SetAuthoredSupport(data, d, air); err != nil {
		t.Fatal(err)
	}
	if AuthoredSupportRows(d) != 4 {
		t.Fatal("reselection did not debit on top of source retained cost")
	}
	mixed, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	if mixed.Support.Mode != 2 || len(mixed.Support.Rows) != 4 || mixed.Support.Rows[0] != 76 || mixed.Support.ControllerKind(77) != 0x19 || mixed.Support.ControllerKind(79) != 0x11 || mixed.Support.LastControllerKind() != 0x11 {
		t.Fatal("source unwritten holes/actual controller identity", mixed.Support)
	}
	if !reflect.DeepEqual(mixed.Objects[76:79], prior.Compiled.Objects[76:79]) {
		t.Fatal("source hole rows overwritten")
	}
	if err = ValidateSupportInitialization(mixed); err != nil {
		t.Fatal(err)
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
	if _, err = CompileAuthoredMission(data, loaded, in); err != nil || !reflect.DeepEqual(frozen, loaded.Compiled) {
		t.Fatal("retained memory changed on BEGIN", err)
	}
	loaded.SupportMemory.Prior.Compiled.Objects[79].Raw[11]++
	if ValidateAuthored(data, loaded) == nil {
		t.Fatal("forged prior source memory accepted")
	}
}
