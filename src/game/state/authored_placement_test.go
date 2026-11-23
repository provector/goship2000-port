package state

import (
	"encoding/binary"
	"testing"
)

func TestNewPlacementUsesFullPrivateFormationTerrainFlags(t *testing.T) {
	data := authoredTestData(t)
	d, _ := authoredReady(t, data)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 2, true)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	w, err := AuthoredEditorWorld(data, d.World)
	if err != nil {
		t.Fatal(err)
	}
	var o AuthoredOption
	for _, candidate := range opts {
		flags := binary.LittleEndian.Uint32(w.FMHD[candidate.Formation*39+35:])
		if flags&0x80000 != 0 && flags&0x00200400 == 0 {
			o = candidate
			break
		}
	}
	if o.SelectionProfile != AuthoredSelectionProfileP5 {
		t.Fatal("new caller not versioned")
	}
	context, err := NewAuthoredEditorContext(data, d.World)
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	for cy := 5; cy < 59 && !checked; cy++ {
		for cx := 5; cx < 59 && !checked; cx++ {
			tile := w.Grid[cx+64*cy]
			land := w.LAND[tile&63]
			if tile&0x80 != 0 || land&1 == 0 || land&16 != 0 {
				continue
			}
			p, e := context.Preview(d, "defender", 0, o, cx*24+12, cy*18+9)
			if e != nil {
				continue
			}
			if p.X == cx*24+12 && p.Y == cy*18+9 {
				t.Fatal("5FBC was replaced with cursor-terrain6040")
			}
			if w.LAND[w.Grid[p.X/24+64*(p.Y/18)]&63]&17 != 17 {
				t.Fatal("full private80000 eligibility not consumed", p)
			}
			if e = PlaceAuthoredOption(data, d, "defender", 0, o, cx*24+12, cy*18+9); e != nil {
				continue
			}
			q := d.Placements[len(d.Placements)-1]
			if q.X != p.X || q.Y != p.Y || q.SelectionProfile != AuthoredSelectionProfileP5 || d.Compiler != AuthoredCompilerP5 {
				t.Fatal("preview/press/compiler ownership differs", p, q, d.Compiler)
			}
			checked = true
		}
	}
	if !checked {
		t.Fatal("source fixture has no discriminating private80000 terrain")
	}
}
func TestOrdinaryNavalChoiceKeepsActualWaterPlacement(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "Naval source", 1)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := AuthoredEditorWorld(data, d.World)
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range opts {
		if w.OBJL[o.Template*32+10] != 2 || binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])&0x200000 == 0 {
			continue
		}
		if err = PlaceAuthoredOption(data, d, "defender", 0, o, 500, 500); err != nil {
			continue
		}
		p := d.Placements[len(d.Placements)-1]
		if w.LAND[w.Grid[p.X/24+64*(p.Y/18)]&63]&2 == 0 {
			t.Fatal("boat placed on land from cursor instead of source mask", p)
		}
		found = true
		break
	}
	if !found {
		t.Fatal("source naval group not exposed/placed", err)
	}
}
