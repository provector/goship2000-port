package state

import (
	"reflect"
	"testing"
)

func TestAuthoredFilmSourceExportsIndependentEditableDescriptors(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	d.Compiler = AuthoredCompilerP5
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	before := authoredDigest(d)
	if err = ValidateAuthoredFilmSource(data, d, m); err != nil {
		t.Fatal(err)
	}
	x, err := NewAuthoredFromFilmSource(data, d, m, "Editable export")
	if err != nil {
		t.Fatal(err)
	}
	if authoredDigest(d) != before || x.ID == d.ID || x.Initial != nil || x.Compiled == nil || !reflect.DeepEqual(x.Placements, d.Placements) || !reflect.DeepEqual(x.Compiled.Objects, d.Compiled.Objects) {
		t.Fatal("Export changed source, lost actual descriptors, or repeated compilation")
	}
	if _, err = CompileAuthoredMission(data, x, in); err != nil {
		t.Fatal("unchanged Export BEGIN", err)
	}
	opts, _, err := AuthoredEditorOptions(data, x, "defender", 0, 10, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	if err = PlaceAuthoredOption(data, x, "defender", 0, opts[0], 1100, 800); err != nil {
		t.Fatal("editable Export placement", err)
	}
	if x.Compiled != nil || len(x.Placements) != len(d.Placements)+1 || authoredDigest(d) != before {
		t.Fatal("editing Export changed film source or kept stale realization")
	}
	if _, err = CompileAuthoredMission(data, x, in); err != nil {
		t.Fatal("edited Export compile", err)
	}
	bad, _ := CloneAuthored(d)
	bad.Compiled.Objects[79].Raw[11]++
	if ValidateAuthoredFilmSource(data, bad, m) == nil {
		t.Fatal("changed frozen source admitted")
	}
	changed := *m
	changed.Objects = append([]MissionObject(nil), m.Objects...)
	changed.Objects[79].Raw[11]++
	if ValidateAuthoredFilmSource(data, d, &changed) == nil {
		t.Fatal("different launch admitted")
	}
}

func TestVisibleUnsupportedOptionsRemainPreciseAndCannotBeForged(t *testing.T) {
	data := authoredTestData(t)
	found := 0
	for _, world := range data.Worlds {
		if world.WorldName == "EUROX.WRL" || world.WorldName == "GULFX.WRL" {
			continue // These are the independently declared Training-only worlds.
		}
		d, err := NewAuthoredDocument(data, world.WorldName, "Known limits", 1)
		if err != nil {
			t.Fatal(err)
		}
		before := AuthoredRevision(d)
		for _, formation := range []bool{false, true} {
			opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, formation)
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range opts {
				if o.SelectionProfile == AuthoredSelectionProfileR23 {
					// R23 admits these choices under a new typed descriptor;
					// a historical P5 descriptor retains its old precise guard.
					legacy := AuthoredPlacement{Role: "defender", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: AuthoredSelectionProfileP5}
					if authoredDefenderAdmission(data, &world, legacy) == nil {
						t.Fatal("historical enemy-air guard lost", o.Name)
					}
					found++
					continue
				}
				if o.Unavailable == "" {
					continue
				}
				found++
				if err = PlaceAuthoredOption(data, d, "defender", 0, o, 500, 500); err == nil {
					t.Fatal("unimplemented option placed", o.Name)
				}
				o.Unavailable = ""
				if err = PlaceAuthoredOption(data, d, "defender", 0, o, 500, 500); err == nil {
					t.Fatal("caller removed availability restriction", o.Name)
				}
				if AuthoredRevision(d) != before {
					t.Fatal("rejected visible option changed draft or compilation seed")
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("actual unsupported source options were hidden")
	}
}
