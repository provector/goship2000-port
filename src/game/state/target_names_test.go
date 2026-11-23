package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSearchTargetNameUsesActualSingleObjectAndKeepsLegacyRealization(t *testing.T) {
	data := authoredTestData(t)
	path := os.Getenv("GS2000_P6_SEARCH_DOCUMENT")
	if path == "" {
		t.Skip("P6 retained S&D document prerequisite: GS2000_P6_SEARCH_DOCUMENT")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := AuthoredStore{Directory: filepath.Dir(path)}
	d, _, err := s.Load(strings.TrimSuffix(filepath.Base(path), ".g2m"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(d.ID))) != "2221247f1a6b99c3196ca5c9bb12ed2d1397a2dcfec65ffdee00e3f2e4cb9d8d" || d.World != "ANTC2.WRL" || d.Compiled.Objectives["primary"].Name != "Sector" || d.TargetNameProfile != "" {
		t.Fatal("not the identified retained P5 S&D witness")
	}
	in := MissionInputs{MissionType: MissionSimulated, Theatre: TheatreAntarctica, Rank: RankCaptain, Seed: int64(d.Seed)}
	before, _ := CloneAuthored(d)
	old, err := CompileAuthoredMission(data, d, in)
	if err != nil || !reflect.DeepEqual(before, d) || old.Objectives["primary"].Name != "Sector" {
		t.Fatal("old unchanged BEGIN was reinterpreted", err)
	}
	unknown, _ := CloneAuthored(d)
	unknown.TargetNameProfile = "unknown-target-name-profile"
	if EnableAuthoredSourceTargetNames(unknown) || unknown.TargetNameProfile != "unknown-target-name-profile" || ValidateAuthored(data, unknown) == nil {
		t.Fatal("unknown source text interpretation was silently replaced/admitted")
	}
	unknown, _ = CloneAuthored(d)
	unknown.Compiled.TargetNameProfile = "unknown-target-name-profile"
	if ValidateAuthored(data, unknown) == nil {
		t.Fatal("unknown frozen text interpretation admitted")
	}
	legacyFresh, _ := CloneAuthored(d)
	legacyFresh.Compiled = nil
	legacy, err := CompileAuthoredMission(data, legacyFresh, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, secondary := range []bool{false, true} {
		q, _ := CloneAuthored(d)
		role := "primary"
		if secondary {
			role = "secondary"
			for i := range q.Placements {
				if q.Placements[i].Role == "primary" {
					q.Placements[i].Role = "secondary"
				} else if q.Placements[i].Role == "secondary" {
					q.Placements[i].Role = "primary"
				}
			}
		}
		if !EnableAuthoredSourceTargetNames(q) || q.Compiled != nil {
			t.Fatal("explicit new compilation did not opt in")
		}
		m, err := CompileAuthoredMission(data, q, in)
		if err != nil {
			t.Fatal(err)
		}
		goal := m.Objectives[role]
		if goal.Name != "KURASSIER" || goal.RawKind != 1 || goal.DisplayKind != 3 || goal.Point == goal.OriginalPoint {
			t.Fatal("unit/search identity or raw/mapped domain changed", goal)
		}
		ri := 2
		if secondary {
			ri = 3
		}
		if !strings.HasSuffix(missionCString(m.Targets.Records[ri][:32]), "S&D KURASSIER") {
			t.Fatal("source TARGET suffix not corrected", missionCString(m.Targets.Records[ri][:32]))
		}
		if !secondary && (!reflect.DeepEqual(m.Objects, legacy.Objects) || !reflect.DeepEqual(m.Paths, legacy.Paths) || !reflect.DeepEqual(m.Conditions, legacy.Conditions) || !reflect.DeepEqual(m.Draws, legacy.Draws) || !bytes.Equal(m.Targets.Records[2][32:], legacy.Targets.Records[2][32:])) {
			t.Fatal("text fix changed logical placement/target operands/RNG")
		}
		store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
		if _, err = store.Save(q, ""); err != nil {
			t.Fatal(err)
		}
		loaded, _, err := store.Load(q.ID)
		if err != nil {
			t.Fatal(err)
		}
		frozen, _ := CloneAuthored(loaded)
		if _, err = CompileAuthoredMission(data, loaded, in); err != nil || !reflect.DeepEqual(frozen, loaded) {
			t.Fatal("new text interpretation changed at reopen/BEGIN", err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("retained witness changed", err)
	}
}

func TestSingleTargetNameOriginalBranchBoundaries(t *testing.T) {
	data := authoredTestData(t)
	w, err := AuthoredWorld(data, "ANTC2.WRL")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		kind, count, sourceKind int
		mandatory, profile      bool
		want                    bool
	}{
		{0, 1, 3, false, true, true}, {4, 1, 3, false, true, true},
		{6, 1, 3, false, true, false}, {1, 1, 3, false, true, false},
		{0, 0, 3, false, true, false}, {0, 2, 3, false, true, false},
		{0, 1, 15, false, true, true}, {0, 1, 16, false, true, false},
		{0, 1, 255, false, true, false}, {0, 1, 16, true, true, true},
		{0, 1, 3, false, false, false},
	} {
		q := *w
		q.OBJL = append([]byte(nil), w.OBJL...)
		q.OBJL[51*32+10] = byte(v.sourceKind)
		g, err := authoredPopulation(&q, authoredRNG(1))
		if err != nil {
			t.Fatal(err)
		}
		g.objects = make([]MissionObject, 1)
		copy(g.objects[0].Raw[15:27], "KURASSIER")
		if v.mandatory {
			g.forms[185].Flags |= 0x4000
		} else {
			g.forms[185].Flags &^= 0x4000
		}
		d := &AuthoredDocument{}
		if v.profile {
			d.TargetNameProfile = AuthoredTargetNameProfile
		}
		p := AuthoredPlacement{SelectionProfile: AuthoredSelectionProfileP5, Role: "primary", Formation: 185, Component: 663, Template: 51, Kind: v.kind}
		name, replaced, err := authoredSingleTargetName(d, g, p, 0, v.count)
		if err != nil || replaced != v.want || replaced && name != "KURASSIER" {
			t.Fatal(v, name, replaced, err)
		}
	}
}
