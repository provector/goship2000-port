package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"testing"
)

// Independent source tally calculator: parse literal FRMD byte2/id255 spans,
// apply20F4's category cap9, and source71 minus consumed rows/header/reservations.
// It deliberately does not call AuthoredComponents/Budget/DefenderCounts.
func sourceTally(t *testing.T, data *assetdata.MissionGenerationData, d *AuthoredDocument) (int, [10]int, int) {
	t.Helper()
	raw, err := AuthoredWorld(data, d.World)
	if err != nil {
		t.Fatal(err)
	}
	private := buildAuthoredEditorWorld(raw)
	referenced := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			referenced[p.Reference] = true
		}
	}
	used := 0
	counts := [10]int{}
	ordinary := 0
	add := func(p AuthoredPlacement) {
		if p.Entry != nil {
			return
		}
		w := raw
		if p.SelectionProfile == AuthoredSelectionProfile || p.SelectionProfile == AuthoredSelectionProfileP5 {
			w = private
		}
		n := 1
		if w.FRMD[p.Component*8+2] == 1 {
			for at := p.Component + 1; at < len(w.FRMD)/8 && w.FRMD[at*8] == 255; at++ {
				if p.SelectionProfile != "" && w.FRMD[at*8+2] != 1 {
					break
				}
				n++
			}
		}
		used += n
		for j := 0; j < n; j++ {
			model := int(w.FRMD[(p.Component+j)*8+1])
			cat := int(data.Tables.EditorObjectTypes[model*27+1])
			if cat > 9 {
				cat = 9
			}
			counts[cat]++
		}
		if p.Kind == 2 && (p.Role == "primary" || p.Role == "secondary") {
			used++
		}
	}
	for _, p := range d.Placements {
		if referenced[p.ID] {
			continue
		}
		if p.Role == "defender" {
			ordinary++
		}
		add(p)
	}
	support := func(s *AuthoredSupport) {
		if s == nil {
			return
		}
		n := 0
		if s.Kind == 0x19 {
			n = 3
		}
		if s.Kind == 0x11 {
			n = 1
		}
		used += n
		for j := 0; j < n; j++ {
			model := int(raw.FRMD[(s.Component+j)*8+1])
			counts[min(9, int(data.Tables.EditorObjectTypes[model*27+1]))]++
		}
	}
	support(d.Support)
	if d.SupportMemory != nil {
		for _, s := range d.SupportMemory.Reservations {
			q := s
			support(&q)
		}
	}
	return 71 - used, counts, ordinary
}
func checkTally(t *testing.T, data *assetdata.MissionGenerationData, d *AuthoredDocument) {
	t.Helper()
	want, counts, ordinary := sourceTally(t, data, d)
	got, err := AuthoredBudget(data, d)
	if got != want || (err != nil) != (want < 0) {
		t.Fatal("source remaining", got, want, err)
	}
	actual, e := AuthoredDefenderCounts(data, d)
	if e != nil || actual != counts {
		t.Fatal("source categories", actual, counts, e)
	}
	if authoredOrdinaryDescriptorCount(d) != ordinary {
		t.Fatal("C018 counts provenance carrier", authoredOrdinaryDescriptorCount(d), ordinary)
	}
}
func gulfReady(t *testing.T, data *assetdata.MissionGenerationData) (*AuthoredDocument, MissionInputs) {
	t.Helper()
	d, e := NewAuthoredDocument(data, "GULF5.WRL", "Counter source", 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, q := range []struct {
		r       string
		k, x, y int
	}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
		opts, e := AuthoredOptions(data, d, q.r, q.k)
		if e != nil || len(opts) == 0 {
			t.Fatal(e)
		}
		if e = PlaceAuthored(data, d, q.r, q.k, opts[0].Formation, q.x, q.y); e != nil {
			t.Fatal(e)
		}
	}
	return d, MissionInputs{MissionType: MissionSimulated, Theatre: TheatrePersianGulf, Rank: RankCaptain, PilotName: "Counter source", Seed: 1}
}
func TestGoalTransferIsNotOrdinaryCountOrDeleteTarget(t *testing.T) {
	data := authoredTestData(t)
	d, in := gulfReady(t, data)
	opts, _, e := AuthoredEditorOptions(data, d, "defender", 0, 10, true)
	if e != nil {
		t.Fatal(e)
	}
	var hq AuthoredOption
	for _, o := range opts {
		if o.Name == "Headquarters" {
			hq = o
			break
		}
	}
	if hq.Name == "" {
		t.Fatal("source HQ required")
	}
	if e = PlaceAuthoredOption(data, d, "defender", 0, hq, 400, 400); e != nil {
		t.Fatal(e)
	}
	unit := d.Placements[len(d.Placements)-1]
	ordinary := authoredOrdinaryDescriptorCount(d)
	before, _ := AuthoredBudget(data, d)
	without, _ := CloneAuthored(d)
	without.Placements = nil
	for _, p := range d.Placements {
		if p.Role != "primary" {
			without.Placements = append(without.Placements, p)
		}
	}
	noGoal, _, _ := sourceTally(t, data, without)
	withGoal, _, _ := sourceTally(t, data, d)
	oldPrimaryCost := noGoal - withGoal
	if e = PlaceAuthored(data, d, "primary", 4, unit.Formation, unit.X, unit.Y); e != nil {
		t.Fatal(e)
	}
	after, _ := AuthoredBudget(data, d)
	//The HQ was already charged when placed. Its transfer to the goal
	//does not charge it again; only the former independent goal is credited.
	//Source1B35 decrements C018 without the1584 refund/debit pair.
	if after != before+oldPrimaryCost || authoredOrdinaryDescriptorCount(d) != ordinary-1 || AuthoredSourceOrdinary(d, unit.ID) {
		t.Fatal("source1B35 transfer", before, after, ordinary, authoredOrdinaryDescriptorCount(d), "oldPrimaryCost", oldPrimaryCost, "sourcebefore", withGoal, "without", noGoal)
	}
	checkTally(t, data, d)
	raw, _ := json.Marshal(d)
	if e = RemoveAuthoredSourceP7(data, d, unit.ID); e == nil {
		t.Fatal("objective carrier deleted as ordinary")
	}
	unchanged, _ := json.Marshal(d)
	if string(raw) != string(unchanged) {
		t.Fatal("rejected deletion mutated draft")
	}
	if _, e = AuthoredBulkDelete(data, d, 18); e != nil {
		t.Fatal(e)
	}
	checkTally(t, data, d)
	if _, e = CompileAuthoredMission(data, d, in); e != nil {
		t.Fatal("retained goal lost through source bulkDelete", e)
	}
	//Reassign primary back to independent pickup restores the retained source
	//ordinary row without changing its stable identity or inventing a new row.
	o, e := AuthoredOptions(data, d, "primary", 7)
	if e != nil || len(o) == 0 {
		t.Fatal(e)
	}
	if e = PlaceAuthored(data, d, "primary", 7, o[0].Formation, 600, 600); e != nil {
		t.Fatal(e)
	}
	if !AuthoredSourceOrdinary(d, unit.ID) {
		t.Fatal("goal reassignment failed to restore source row")
	}
	checkTally(t, data, d)
}
func TestAllFillDeleteSourceAccountingWithSupportAndOptionalFARP(t *testing.T) {
	data := authoredTestData(t)
	base, in := gulfReady(t, data)
	supports, e := AuthoredSupportOptions(data, base)
	if e != nil {
		t.Fatal(e)
	}
	for _, support := range supports {
		for _, withFARP := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/farp%t", support.Name, withFARP), func(t *testing.T) {
				d, _ := CloneAuthored(base)
				if withFARP {
					opts, e := AuthoredOptions(data, d, "FARP", 0)
					if e != nil || len(opts) == 0 {
						t.Fatal(e)
					}
					if e = PlaceAuthored(data, d, "FARP", 0, opts[0].Formation, 1000, 600); e != nil {
						t.Fatal(e)
					}
				}
				if e = SetAuthoredSupport(data, d, support); e != nil {
					t.Fatal(e)
				}
				opts, _, e := AuthoredEditorOptions(data, d, "defender", 0, 10, true)
				if e != nil {
					t.Fatal(e)
				}
				for _, o := range opts {
					if o.Name == "Headquarters" {
						if e = PlaceAuthoredOption(data, d, "defender", 0, o, 400, 400); e != nil {
							t.Fatal(e)
						}
						break
					}
				}
				checkTally(t, data, d)
				original, _ := json.Marshal(d)
				for slot := 0; slot < 6; slot++ {
					q, _ := CloneAuthored(d)
					if _, e = AuthoredInteractiveFill(data, q, slot); e != nil {
						t.Fatal("Fill", slot, e)
					}
					checkTally(t, data, q)
					if n, e := AuthoredBulkDelete(data, q, 0); e != nil {
						t.Fatal(n, e)
					}
					checkTally(t, data, q)
					if _, e = CompileAuthoredMission(data, q, in); e != nil {
						t.Fatal("Fill then source excess removal compile", slot, e)
					}
					for del := 0; del < 19; del++ {
						r, _ := CloneAuthored(q)
						_, e = AuthoredBulkDelete(data, r, del)
						if e != nil {
							raw, _ := json.Marshal(r)
							frozen, _ := json.Marshal(q)
							if string(raw) != string(frozen) {
								t.Fatal("failed Delete mutated", del, e)
							}
							continue
						}
						checkTally(t, data, r)
					}
				}
				now, _ := json.Marshal(d)
				if string(original) != string(now) {
					t.Fatal("model-only accounting mutated source draft")
				}
			})
		}
	}
}
func TestTransferInstructionsAndFreshCounterSource(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_AUDIO_ORIGINAL")
	if dir == "" {
		t.Skip("hash-bound original GS fixture required")
	}
	b, e := os.ReadFile(filepath.Join(dir, "GS.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, want := range map[int][]byte{0x1afd: {0x8b, 0x7e, 0x0a, 0xb9, 0x05, 0x00, 0xf3, 0xa5, 0xa4}, 0x1b35: {0xff, 0x0e, 0x18, 0xc0}, 0x1605: {0x28, 0x06, 0x98, 0x2b}, 0x165b: {0x00, 0x06, 0x98, 0x2b}, 0x4fc2: {0xc6, 0x06, 0x98, 0x2b, 0x47}} {
		if !bytes.Equal(b[0x287c0+ip:0x287c0+ip+len(want)], want) {
			t.Fatal("source transfer/counter instruction", ip)
		}
	}
}

func TestNearestGoalUsesActualSourceOrdinaryOwnership(t *testing.T) {
	data := authoredTestData(t)
	d, _ := gulfReady(t, data)
	opts, _, e := AuthoredEditorOptions(data, d, "defender", 0, 10, true)
	if e != nil {
		t.Fatal(e)
	}
	var hq AuthoredOption
	for _, o := range opts {
		if o.Name == "Headquarters" {
			hq = o
			break
		}
	}
	if hq.Name == "" {
		t.Fatal("source HQ required")
	}
	units := []AuthoredPlacement{}
	for _, at := range [][2]int{{300, 300}, {1000, 700}} {
		if e = PlaceAuthoredOption(data, d, "defender", 0, hq, at[0], at[1]); e != nil {
			t.Fatal(e)
		}
		units = append(units, d.Placements[len(d.Placements)-1])
	}
	for i, role := range []string{"primary", "secondary"} {
		u := units[i]
		if e = PlaceAuthored(data, d, role, 4, u.Formation, u.X, u.Y); e != nil {
			t.Fatal(e)
		}
	}
	nearest, e := NearestAuthoredGoalForRole(data, d, units[1].X, units[1].Y, 4, "primary")
	if e != nil || nearest.ID == units[1].ID {
		t.Fatal("other goal's provenance is not a source ordinary row", nearest, e)
	}
	own, e := NearestAuthoredGoalForRole(data, d, units[1].X, units[1].Y, 4, "secondary")
	if e != nil || own.ID != units[1].ID {
		t.Fatal("current goal failed source restoration before selection", own, e)
	}
	checkTally(t, data, d)
}
