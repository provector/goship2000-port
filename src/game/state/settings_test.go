package state

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestPilotPreflightOriginalDefaultsAndLegacyMapping(t *testing.T) {
	if got := defaultPreflightSettings(); got.Values() != [7]int{} {
		t.Fatal("original new record is cleared; seven preferences start at ordinal zero", got)
	}
	// Distinct valid ordinals expose the original record-byte order rather than
	// accidentally testing the ANIM key order against itself.
	raw := make([]byte, LegacyRecordSize)
	for offset, value := range map[int]byte{0x5d: 3, 0x5e: 1, 0x5f: 2, 0x60: 1, 0x61: 0, 0x62: 1, 0x63: 0} {
		raw[offset] = value
	}
	p := &Pilot{LegacyRaw: raw}
	got, err := PreflightFromPilot(p)
	want := PreflightSettings{CPGControls: 2, EnemyQuality: 3, FlightModel: 1, Landings: 1, Avoidance: 0, Wind: 1, Visibility: 0}
	if err != nil || got != want {
		t.Fatal("C01db..0241 raw record mapping", got, err)
	}
	before := append([]byte(nil), raw...)
	typed := PreflightSettings{CPGControls: 1, EnemyQuality: 2, Visibility: 1}
	if err = CopyPilotPreflight(p, typed); err != nil {
		t.Fatal(err)
	}
	got, err = PreflightFromPilot(p)
	if err != nil || got != typed || !bytes.Equal(before, p.LegacyRaw) {
		t.Fatal("typed preference ownership must not overwrite preserved import bytes", got, err)
	}
	typed.EnemyQuality = 0
	if p.Preflight.EnemyQuality != 2 {
		t.Fatal("saved preferences alias local/session edit")
	}
}

func TestPilotPreflightValidationDoesNotResetUnknownOrDamagedState(t *testing.T) {
	if _, err := PreflightFromPilot(nil); err == nil {
		t.Fatal("nil pilot accepted")
	}
	if _, err := PreflightFromPilot(&Pilot{}); err == nil {
		t.Fatal("missing legacy preferences silently converted into defaults")
	}
	p := &Pilot{Preflight: &PreflightSettings{EnemyQuality: 99}, LegacyRaw: make([]byte, LegacyRecordSize)}
	if _, err := PreflightFromPilot(p); err == nil {
		t.Fatal("damaged authoritative typed settings fell back to old import")
	}
	valid := defaultPreflightSettings()
	p.Preflight = &valid
	if err := CopyPilotPreflight(p, PreflightSettings{Wind: 2}); err == nil {
		t.Fatal("invalid setting accepted")
	}
	if *p.Preflight != valid {
		t.Fatal("validation failure partially changed preferences")
	}
	if err := CopyPilotPreflight(nil, valid); err == nil {
		t.Fatal("nil copy target accepted")
	}
}

func TestPilotPreflightPreferencesIndependentAcrossSaveReload(t *testing.T) {
	store := saveTestStore(t)
	state := store.State()
	first, second := saveTestPilot(t, "First Pilot"), saveTestPilot(t, "Second Pilot")
	a := PreflightSettings{CPGControls: 1, EnemyQuality: 3, Wind: 1}
	b := PreflightSettings{CPGControls: 2, EnemyQuality: 1, FlightModel: 1, Visibility: 1}
	if err := CopyPilotPreflight(first, a); err != nil {
		t.Fatal(err)
	}
	if err := CopyPilotPreflight(second, b); err != nil {
		t.Fatal(err)
	}
	state.Slots[0], state.Slots[1] = first, second
	state.SelectedPilotID = first.ID
	if err := store.Commit(state); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	saved := reopened.State()
	if got, err := PreflightFromPilot(saved.SelectedPilot()); err != nil || got != a {
		t.Fatal(got, err)
	}
	saved.SelectedPilotID = second.ID
	if err = reopened.Commit(saved); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded := again.State()
	if got, err := PreflightFromPilot(loaded.SelectedPilot()); err != nil || got != b {
		t.Fatal(got, err)
	}
	if got, err := PreflightFromPilot(loaded.Slots[0]); err != nil || got != a {
		t.Fatal("switching pilot changed first pilot preferences", got, err)
	}
	// The test creates ordinary editable pilot JSON only. No gameplay backup is
	// manufactured by selecting or changing these UI preferences.
	for _, p := range []*Pilot{first, second} {
		if matches, err := filepath.Glob(filepath.Join(store.Directory, "pilots", p.ID+".backup.json")); err != nil || len(matches) != 0 {
			t.Fatal("preference commit created a gameplay backup", matches, err)
		}
	}
}
