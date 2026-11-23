package state

import (
	"fmt"
	"reflect"
	"testing"
)

func TestGeneratedEditorRoundTrip(t *testing.T) {
	a := missionReadinessAssets(t)
	for _, kind := range []MissionType{MissionTraining, MissionSingle, MissionFlight} {
		for _, seed := range []int64{1, 2, 3, 4, 5, 7} {
			t.Run(fmt.Sprintf("%s-%d", kind, seed), func(t *testing.T) {
				in := MissionInputs{MissionType: kind, Theatre: TheatreCentralEurope, Rank: RankCaptain, PilotName: "Flight sample", Seed: seed}
				m, err := GenerateMission(a.Generation, in)
				if err != nil {
					t.Fatal(err)
				}
				if err = PrepareMissionTargets(a, m); err != nil {
					t.Fatal(err)
				}
				old := authoredDigest(m)
				d, err := NewGeneratedDocument(a.Generation, m.GenerationIndex(), m, "Flight sample")
				if err != nil {
					t.Fatal(err)
				}
				if old != authoredDigest(m) {
					t.Fatal("capture changed preparation")
				}
				in.MissionType = MissionSimulated
				restore := func(d *AuthoredDocument) *MissionData {
					q, e := CompileAuthoredMission(a.Generation, d, in)
					if e != nil {
						t.Fatal(e)
					}
					return q
				}
				q := restore(d)
				if !reflect.DeepEqual(q.Objects, m.Objects) || !reflect.DeepEqual(q.Paths, m.Paths) || !reflect.DeepEqual(q.Objectives, m.Objectives) || !reflect.DeepEqual(q.Targets, m.Targets) || !reflect.DeepEqual(q.Support, m.Support) {
					t.Fatal("initial owners differ")
				}
				t.Logf("%s, %d groups, %d path points, flags %04x, Support %t", m.World, len(d.Placements), len(q.Paths), q.Conditions.Flags, q.Support != nil)
				store := AuthoredStore{t.TempDir()}
				if _, err = store.SaveExport(d); err != nil {
					t.Fatal(err)
				}
				d, rev, err := store.Load(d.ID)
				if err != nil {
					t.Fatal(err)
				}
				for i, p := range d.Placements {
					if p.Role == "primary" {
						d.Placements[i].X += 1
						d.Compiled = nil
						changed := restore(d)
						if changed.Objectives["primary"].Point.X != m.Objectives["primary"].Point.X+341 {
							t.Fatal("objective not moved")
						}
						if reflect.DeepEqual(changed.Objects, m.Objects) && reflect.DeepEqual(changed.Paths, m.Paths) {
							t.Fatal("group stayed behind")
						}
						if _, err = store.Save(d, rev); err != nil {
							t.Fatal(err)
						}
						reopened, _, err := store.Load(d.ID)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(restore(reopened).Objectives, changed.Objectives) {
							t.Fatal("saved edit lost")
						}
						return
					}
				}
				t.Fatal("no editable primary")
			})
		}
	}
}

// Remove a whole placement before an objective and another route. Verify the
// remapped physical indices using independent slices of the initial mission.
func TestGeneratedDeleteRemapsRoutesAndTargets(t *testing.T) {
	a := missionReadinessAssets(t)
	in := MissionInputs{MissionType: MissionSingle, Theatre: TheatreCentralEurope, Rank: RankCaptain, Seed: 3}
	m, err := GenerateMission(a.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if err = PrepareMissionTargets(a, m); err != nil {
		t.Fatal(err)
	}
	d, err := NewGeneratedDocument(a.Generation, m.GenerationIndex(), m, "Roads")
	if err != nil {
		t.Fatal(err)
	}
	groups := m.GenerationIndex().Groups
	var at int
	for i, p := range d.Placements {
		if p.Role == "defender" && !p.Builtin && (d.Generated.GoalOwners["tertiary"] != p.ID) {
			at = i
			break
		}
	}
	p := d.Placements[at]
	cut := groups[at]
	if err = RemoveAuthoredSourceP7(a.Generation, d, p.ID); err != nil {
		t.Fatal(err)
	}
	in.MissionType = MissionSimulated
	q, err := CompileAuthoredMission(a.Generation, d, in)
	if err != nil {
		t.Fatal(err)
	}
	for name, g := range m.Objectives {
		want := g.ObjectIndex
		if want >= cut.First+cut.Count {
			want -= cut.Count
		}
		if q.Objectives[name].ObjectIndex != want {
			t.Fatalf("%s points at row %d, want %d", name, q.Objectives[name].ObjectIndex, want)
		}
	}
	if len(q.Paths) != len(m.Paths)-cut.PathCount || q.Objects[79].Raw[10] != 0xff {
		t.Fatal("route or vacant slot ownership")
	}
	for i := cut.First; i < 80-cut.Count; i++ {
		if q.Objects[i].Raw[12] != m.Objects[i+cut.Count].Raw[12] {
			t.Fatal("member moved independently", i)
		}
	}
}

func TestGeneratedSupportTransaction(t *testing.T) {
	a := initializationAssets(t).Missions
	in := MissionInputs{MissionType: MissionFlight, Theatre: TheatreCentralEurope, Rank: RankCaptain, Seed: 1}
	m, err := GenerateMission(a.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if err = PrepareMissionTargets(a, m); err != nil {
		t.Fatal(err)
	}
	d, err := NewGeneratedDocument(a.Generation, m.GenerationIndex(), m, "Support")
	if err != nil {
		t.Fatal(err)
	}
	if d.Support == nil {
		t.Fatal("missing initial support")
	}
	opts, err := AuthoredSupportOptions(a.Generation, d)
	if err != nil {
		t.Fatal(err)
	}
	in.MissionType = MissionSimulated
	if err = CommitAuthoredSupportChoice(a.Generation, d, opts[0], AuthoredSupportEditFingerprint(d)); err != nil {
		t.Fatal(err)
	}
	cleared, err := CompileAuthoredMission(a.Generation, d, in)
	if err != nil || cleared.Support != nil {
		t.Fatal("cancelled Support still active", err)
	}
	if err = CommitAuthoredSupportChoice(a.Generation, d, opts[len(opts)-1], AuthoredSupportEditFingerprint(d)); err != nil {
		t.Fatal(err)
	}
	q, err := CompileAuthoredMission(a.Generation, d, in)
	if err != nil || q.Support == nil {
		t.Fatal("missing replacement", err)
	}
	if err = ValidateSupportInitialization(q); err != nil {
		t.Fatal(err)
	}
}
