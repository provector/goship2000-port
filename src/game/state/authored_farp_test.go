package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestFARPLiftRefundAndPreservationBeforeGoals(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "FARP lift", 1)
	if err != nil {
		t.Fatal(err)
	}
	o, err := CheckAuthoredFARPArm(data, d)
	if err != nil {
		t.Fatal(err)
	}
	if err = PlaceAuthored(data, d, "FARP", 0, o.Formation, 1000, 600); err != nil {
		t.Fatal(err)
	}
	old := d.Placements[len(d.Placements)-1]
	remain, _ := AuthoredBudget(data, d)
	w, _ := AuthoredWorld(data, d.World)
	terrain := append([]byte(nil), w.Grid...)
	seed := d.Seed
	lifted, err := LiftAuthoredFARP(data, d)
	if err != nil || lifted == nil || lifted.ID != old.ID {
		t.Fatal(lifted, err)
	}
	rows, err := AuthoredComponents(data, d, *lifted)
	if err != nil {
		t.Fatal(err)
	}
	after, err := AuthoredBudget(data, d)
	if err != nil || after != remain+len(rows) {
		t.Fatal("cost was not refunded exactly once", remain, after, rows, err)
	}
	if d.Compiled != nil || d.Seed != seed || !bytes.Equal(terrain, w.Grid) {
		t.Fatal("lift changed immutable terrain/stream or retained compile")
	}
	checkTally(t, data, d)
	before, _ := json.Marshal(d)
	lifted, err = LiftAuthoredFARP(data, d)
	current, _ := json.Marshal(d)
	if err != nil || lifted != nil || !bytes.Equal(before, current) {
		t.Fatal("repeated lift mutated absent FARP", err)
	}
	//No guessed eligibility: a descriptor relabelled FARP is rejected intact.
	if err = PlaceAuthored(data, d, "FARP", 0, o.Formation, 1000, 600); err != nil {
		t.Fatal(err)
	}
	d.Placements[len(d.Placements)-1].Formation = 0
	before, _ = json.Marshal(d)
	if _, err = LiftAuthoredFARP(data, d); err == nil {
		t.Fatal("wrong source owner accepted")
	}
	current, _ = json.Marshal(d)
	if !bytes.Equal(before, current) {
		t.Fatal("failed lift damaged draft")
	}
	for _, fn := range []func(*AuthoredDocument) error{
		func(d *AuthoredDocument) error { _, e := LiftAuthoredFARP(data, d); return e },
		func(d *AuthoredDocument) error { _, e := CheckAuthoredFARPArm(data, d); return e },
	} {
		if err := fn(nil); err == nil {
			t.Fatal("nil document accepted")
		}
		q, _ := CloneAuthored(d)
		q.Dependency = fmt.Sprintf("%064x", 1)
		before, _ := json.Marshal(q)
		if err := fn(q); err == nil {
			t.Fatal("unknown source dependency accepted")
		}
		after, _ := json.Marshal(q)
		if !bytes.Equal(before, after) {
			t.Fatal("dependency rejection mutated draft")
		}
	}
}
func TestFARPArmSourceResourceEqualityAndCapacity(t *testing.T) {
	data := authoredTestData(t)
	d, err := NewAuthoredDocument(data, "GULF5.WRL", "FARP boundary", 1)
	if err != nil {
		t.Fatal(err)
	}
	//Independent boundary fixtures isolate source6C4C's strict9D42 threshold.
	q := *data
	q.Tables = data.Tables
	q.Tables.EditorModelCosts = make([]uint16, len(data.Tables.EditorModelCosts))
	q.Tables.EditorWorldBaseCosts = append([]uint16(nil), data.Tables.EditorWorldBaseCosts...)
	w, _ := AuthoredWorld(data, d.World)
	wi := int(data.Tables.EditorTheatreOffsets[w.Theatre]) + w.Suffix
	for _, v := range []uint16{0x9d41, 0x9d42, 0x9d43} {
		q.Tables.EditorWorldBaseCosts[wi] = v
		before, _ := json.Marshal(d)
		_, err := CheckAuthoredFARPArm(&q, d)
		if (err != nil) != (v >= 0x9d42) {
			t.Fatal(v, err)
		}
		after, _ := json.Marshal(d)
		if !bytes.Equal(before, after) {
			t.Fatal("precheck spent or cleared state")
		}
	}
	opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 1, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(opts, err)
	}
	d.Placements = nil // isolate C018 capacity, independently of the encoded80-record guard.
	for i := 0; i < 75; i++ {
		d.Placements = append(d.Placements, AuthoredPlacement{ID: fmt.Sprintf("%032x", 100+i), Role: "defender", Formation: opts[0].Formation, Component: opts[0].Component, Template: opts[0].Template, SelectionProfile: opts[0].SelectionProfile})
	}
	before, _ := json.Marshal(d)
	if _, err = CheckAuthoredFARPArm(data, d); err == nil || !strings.Contains(err.Error(), "75 descriptors") {
		t.Fatal("75 ordinary descriptor precheck ignored", err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("capacity rejection mutated draft")
	}
}
func TestFARPLiftWithSourceSupportAndNearCapacity(t *testing.T) {
	data := authoredTestData(t)
	for mode := 0; mode < 4; mode++ {
		d, _ := gulfReady(t, data)
		opts, err := AuthoredSupportOptions(data, d)
		if err != nil {
			t.Fatal(err)
		}
		if mode > 0 {
			n := 0
			for _, o := range opts {
				if o.Template < 0 {
					continue
				}
				n++
				if n == mode {
					if err = SetAuthoredSupport(data, d, o); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			if d.Support == nil {
				t.Fatal("Support test choice missing", mode)
			}
		}
		o, err := CheckAuthoredFARPArm(data, d)
		if err != nil {
			t.Fatal(err)
		}
		if err = PlaceAuthored(data, d, "FARP", 0, o.Formation, 1000, 600); err != nil {
			t.Fatal(err)
		}
		if _, err = AuthoredInteractiveFill(data, d, 0); err != nil {
			t.Fatal(err)
		}
		remain, _ := AuthoredBudget(data, d)
		before, _ := json.Marshal(d.Support)
		lifted, err := LiftAuthoredFARP(data, d)
		if err != nil || lifted == nil {
			t.Fatal(mode, err)
		}
		rows, _ := AuthoredComponents(data, d, *lifted)
		after, _ := AuthoredBudget(data, d)
		if after != remain+len(rows) {
			t.Fatal(mode, remain, after)
		}
		support, _ := json.Marshal(d.Support)
		if !bytes.Equal(before, support) {
			t.Fatal("lift changed Support")
		}
		checkTally(t, data, d)
	}
}
