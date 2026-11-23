package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestInteractiveFillAllActualChoices(t *testing.T) {
	data := authoredTestData(t)
	for slot := 0; slot < 6; slot++ {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			d, in := authoredReady(t, data)
			before, _ := json.Marshal(d)
			n, err := AuthoredInteractiveFill(data, d, slot)
			if err != nil {
				after, _ := json.Marshal(d)
				if string(before) != string(after) {
					t.Fatal("failed Fill changed draft")
				}
				t.Fatal(err)
			}
			if n == 0 {
				t.Fatal("nonempty source budget produced no placements")
			}
			m, err := CompileAuthoredMission(data, d, in)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("slot%d added%d fullrows%d seed%d", slot, n, len(m.Objects), d.Editor.Stream)
		})
	}
}

func TestDeleteSwapOrderAndIndependentGoalCopy(t *testing.T) {
	data, d := controlFixture()
	//Four literal ordinary source records A/B/C/D. 1934..1972 leaves
	//A/D/C when B is deleted; preserving native slice order A/C/D is wrong.
	def := append([]AuthoredPlacement(nil), d.Placements[:4]...)
	d.Placements = def
	goal := def[1]
	goal.ID = fmt.Sprintf("%032x", 777)
	goal.Role, goal.Kind, goal.Reference = "primary", 0, def[1].ID
	d.Placements = append(d.Placements, goal)
	if err := RemoveAuthoredSource(data, d, def[1].ID); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, i := range authoredOrdinaryIndexes(d) {
		ids = append(ids, d.Placements[i].ID)
	}
	if !reflect.DeepEqual(ids, []string{def[0].ID, def[3].ID, def[2].ID}) {
		t.Fatal(ids)
	}
	got := d.Placements[len(d.Placements)-1]
	goal.Reference = ""
	goal.GoalCopyProfile = AuthoredGoalCopyProfile
	if !reflect.DeepEqual(got, goal) {
		t.Fatal("goal copied record changed", got, goal)
	}
	//Category deletion scans forward, rechecking the last row swapped in.
	data, d = controlFixture()
	d.Placements = append([]AuthoredPlacement(nil), d.Placements[:4]...)
	data.Tables.EditorObjectTypes[1*27+1] = 2
	data.Tables.EditorObjectTypes[2*27+1] = 3
	data.Tables.EditorObjectTypes[3*27+1] = 4
	data.Tables.EditorObjectTypes[4*27+1] = 2
	//Category2 (AA) is source popup12; both first+last are removed.
	want := []string{d.Placements[2].ID, d.Placements[1].ID}
	n, err := AuthoredBulkDelete(data, d, 12)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	ids = nil
	for _, p := range d.Placements {
		ids = append(ids, p.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatal("forward source swap ordering", ids, want)
	}
}

func TestDeleteProtectedLastIsNotSkipped(t *testing.T) {
	for _, slot := range []int{1, 2, 3} {
		data, d := controlFixture()
		d.Editor = &AuthoredEditorState{Profile: AuthoredControlProfile, Stream: 7, FillBoundary: 10}
		before, _ := json.Marshal(d)
		n, err := AuthoredBulkDelete(data, d, slot)
		if slot == 1 && err == nil {
			t.Fatal("unbounded protected suffix not diagnosed")
		}
		if slot > 1 && err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(d)
		if n != 0 || !bytes.Equal(before, after) {
			t.Fatal("protected last skipped", slot, n, err)
		}
	}
}

func TestFillDeleteFillAndSavedRealization(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	prefix, _ := json.Marshal(d.Placements)
	if n, err := AuthoredInteractiveFill(data, d, 2); err != nil || n == 0 {
		t.Fatal(n, err)
	}
	firstStream := d.Editor.Stream
	if n, err := AuthoredBulkDelete(data, d, 1); err != nil || n == 0 {
		t.Fatal(n, err)
	}
	after, _ := json.Marshal(d.Placements)
	if !bytes.Equal(prefix, after) {
		t.Fatal("Delete Filled modified preexisting placements")
	}
	if n, err := AuthoredInteractiveFill(data, d, 4); err != nil || n == 0 {
		t.Fatal(n, err)
	}
	if d.Editor.Stream == firstStream {
		t.Fatal("second Fill did not use its stored stream")
	}
	if _, err := CompileAuthoredMission(data, d, in); err != nil {
		t.Fatal(err)
	}
	store := AuthoredStore{Directory: t.TempDir()}
	rev, err := store.Save(d, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, readRev, err := store.Load(d.ID)
	if err != nil || rev != readRev {
		t.Fatal(readRev, rev, err)
	}
	want := authoredDigest(loaded.Compiled)
	if _, err = CompileAuthoredMission(data, loaded, in); err != nil || want != authoredDigest(loaded.Compiled) {
		t.Fatal("unchanged BEGIN recompiled", err)
	}
	//Invalid replacement is rejected without touching the prior complete file.
	loaded.Placements = nil
	if _, err = CompileAuthoredMission(data, loaded, in); err == nil {
		t.Fatal("invalid mission compiled")
	}
	still, stillRev, err := store.Load(d.ID)
	if err != nil || stillRev != rev || authoredDigest(still.Compiled) != want {
		t.Fatal("failed draft damaged saved mission", err)
	}
}

func TestFillFocusLiteralSourceOperands(t *testing.T) {
	//025C 030C..0336 uses the copied route endpoint's *positive*18 divisor,
	//sets step4, and jumps over the slot's parity/rotation. Start(10,20),
	//destination(40,50), D=30+30/2=45 gives +(2,2), not the transformedY.
	p := AuthoredPlacement{X: 240, Y: (63 - 20) * 18, Kind: 2, Route: &AuthoredRoute{Profile: AuthoredRouteProfile, X: 960, Y: 900}}
	for slot := 1; slot < 6; slot++ {
		center, radius, minimum := authoredFillFocus(p, [2]int{40, 50}, slot, 0)
		if center != ([2]int{12, 22}) || radius != 6 || minimum != 2 {
			t.Fatal(slot, center, radius, minimum)
		}
	}
	//Literal ordinary objective origins exercise separate left/right/forward/
	//rear displacements and minimum/doubled-radius gates, without RNG draws.
	p.Kind, p.Route = 7, nil
	for _, q := range []struct {
		slot int
		want [2]int
	}{{1, [2]int{10, 20}}, {2, [2]int{14, 20}}, {3, [2]int{6, 20}}, {4, [2]int{12, 22}}, {5, [2]int{8, 18}}} {
		center, radius, minimum := authoredFillFocus(p, [2]int{40, 50}, q.slot, 0x400)
		if center != q.want || radius != 16 || minimum != 6 {
			t.Fatal(q.slot, center, radius, minimum)
		}
	}
}

func TestEveryDeleteSparseMixedAndNearCapacity(t *testing.T) {
	for _, ordinary := range []int{0, 12, 72} {
		for slot := 0; slot < 19; slot++ {
			t.Run(fmt.Sprintf("count%d/slot%d", ordinary, slot), func(t *testing.T) {
				data, d := controlFixture()
				special := append([]AuthoredPlacement(nil), d.Placements[12:]...)
				raw := append([]AuthoredPlacement(nil), d.Placements[:12]...)
				d.Placements = nil
				for n := 0; n < ordinary; n++ {
					p := raw[n%len(raw)]
					p.ID = fmt.Sprintf("%032x", 10000+n)
					p.Builtin = false
					if p.Formation == 12 {
						p.Formation, p.Component, p.Template = 1, 1, 1
					}
					d.Placements = append(d.Placements, p)
				}
				if ordinary > 0 {
					special[1].Kind = 4
					special[2].Kind = 6
					special[1].Reference = d.Placements[0].ID
					special[2].Reference = d.Placements[ordinary-1].ID
				}
				d.Placements = append(d.Placements, special...)
				sourceOrdinary := ordinary - min(2, ordinary)
				d.Editor = &AuthoredEditorState{Profile: AuthoredControlProfile, Stream: 19, FillBoundary: sourceOrdinary / 2}
				d.Support = &AuthoredSupport{Profile: AuthoredSupportProfile, Formation: 1, Component: 1, Template: 1, Kind: 0x19} //Three reserved actors remain independent of ordinary Delete.
				beforeSpecial := append([]AuthoredPlacement(nil), special...)
				n, err := AuthoredBulkDelete(data, d, slot)
				if err != nil {
					t.Fatal(err)
				}
				after := authoredOrdinaryIndexes(d)
				if n != ordinary-len(after) {
					t.Fatal("count mismatch", n, ordinary, len(after))
				}
				if d.Support == nil || d.Editor.Stream != 19 {
					t.Fatal("Delete changed unrelated Support/RNG")
				}
				for j := range beforeSpecial {
					found := false
					for _, p := range d.Placements {
						if p.ID == beforeSpecial[j].ID {
							q := beforeSpecial[j]
							if p.Reference == "" {
								if q.Reference != "" {
									q.GoalCopyProfile = AuthoredGoalCopyProfile
								}
								q.Reference = ""
							}
							if !reflect.DeepEqual(p, q) {
								t.Fatal("independent special copy changed", p, q)
							}
							found = true
							break
						}
					}
					if !found {
						t.Fatal("base/goal removed")
					}
				}
				if slot == 2 && n != min(5, sourceOrdinary) || slot == 3 && n != min(1, sourceOrdinary) || slot == 18 && n != sourceOrdinary || slot == 1 && n != sourceOrdinary-sourceOrdinary/2 {
					t.Fatal("literal dispatch count", slot, n, ordinary)
				}
			})
		}
	}
}

func TestSourceDeletePreservesCopiedSearchAndRouteThroughSave(t *testing.T) {
	data := authoredTestData(t)
	for _, kind := range []int{0, 2, 4, 6} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			d, in := authoredReady(t, data)
			w, _ := AuthoredWorld(data, d.World)
			opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 2, true)
			if err != nil {
				t.Fatal(err)
			}
			var option AuthoredOption
			found := false
			for _, o := range opts {
				if binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])&0x180 != 0 && o.Unavailable == "" {
					option = o
					found = true
					break
				}
			}
			if !found {
				t.Fatal("actual moving ground source group missing")
			}
			if err = PlaceAuthoredOption(data, d, "defender", 0, option, 400, 400); err != nil {
				t.Fatal(err)
			}
			unit := d.Placements[len(d.Placements)-1]
			if err = PlaceAuthored(data, d, "primary", kind, unit.Formation, unit.X, unit.Y); err != nil {
				t.Fatal(err)
			}
			if kind == 0 {
				if err = SetAuthoredSearchPoint(data, d, "primary", 600, 600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == 2 {
				reached := false
				for _, at := range [][2]int{{600, 400}, {400, 600}, {500, 500}, {800, 700}} {
					if err = SetAuthoredRoutePoint(data, d, "primary", at[0], at[1]); err == nil {
						reached = true
						break
					}
				}
				if !reached {
					t.Fatal("no actual source planned route", err)
				}
			}
			before, err := CompileAuthoredMission(data, d, in)
			if err != nil {
				t.Fatal(err)
			}
			if err = RemoveAuthoredSource(data, d, unit.ID); err != nil {
				t.Fatal(err)
			}
			goal := AuthoredPlacement{}
			for _, p := range d.Placements {
				if p.Role == "primary" {
					goal = p
				}
			}
			if !authoredDetachedGoal(goal) || goal.Formation != unit.Formation || goal.Component != unit.Component || goal.X != unit.X || goal.Y != unit.Y {
				t.Fatal("source special copy was lost", goal, unit)
			}
			after, err := CompileAuthoredMission(data, d, in)
			if err != nil {
				t.Fatal("legal copied goal rejected", err)
			}
			if before.ObjectHash != after.ObjectHash || !reflect.DeepEqual(before.Paths, after.Paths) || !reflect.DeepEqual(before.Targets, after.Targets) {
				t.Fatal("ordinary removal changed independent source realization")
			}
			store := AuthoredStore{Directory: t.TempDir()}
			if _, err = store.Save(d, ""); err != nil {
				t.Fatal(err)
			}
			loaded, _, err := store.Load(d.ID)
			if err != nil {
				t.Fatal(err)
			}
			frozen, _ := cloneAuthoredRealization(loaded.Compiled)
			again, err := CompileAuthoredMission(data, loaded, in)
			if err != nil || !reflect.DeepEqual(frozen, loaded.Compiled) || again.ObjectHash != after.ObjectHash {
				t.Fatal("detached copied goal BEGIN changed", err)
			}
			for i, p := range loaded.Placements {
				if p.GoalCopyProfile != "" {
					loaded.Placements[i].GoalCopyProfile = "unknown"
					if ValidateAuthored(data, loaded) == nil {
						t.Fatal("unknown detached-copy interpretation accepted")
					}
					break
				}
			}
		})
	}
}
