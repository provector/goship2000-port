package state

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSupportOwnedTransactionCompileSaveAndConflict(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	d.SupportEdits = &AuthoredSupportEdits{Profile: OwnedSupportChoiceV1}
	options, e := AuthoredSupportOptions(data, d)
	if e != nil {
		t.Fatal(e)
	}
	store := AuthoredStore{Directory: t.TempDir()}
	rev := ""
	for _, index := range []int{1, 1, 2, 0, 1, 0, 2, 0} {
		if index >= len(options) {
			t.Fatal("required Support choices absent")
		}
		token := AuthoredSupportEditFingerprint(d)
		if e = CommitAuthoredSupportChoice(data, d, options[index], token); e != nil {
			t.Fatal(index, e)
		}
		if d.SupportMemory != nil {
			t.Fatal("leaked literal reservation")
		}
		if _, e = AuthoredBudget(data, d); e != nil {
			t.Fatal(e)
		}
		m, e := CompileAuthoredMission(data, d, in)
		if e != nil || len(m.Objects) != 80 {
			t.Fatal(index, e)
		}
		if rev, e = store.Save(d, rev); e != nil {
			t.Fatal(e)
		}
		reopened, actual, e := store.Load(d.ID)
		if e != nil || actual != rev || !reflect.DeepEqual(d, reopened) {
			t.Fatal("save/reopen", e)
		}
		before, _ := json.Marshal(d)
		if e = CommitAuthoredSupportChoice(data, d, options[index], AuthoredSupportEditFingerprint(d)); e != nil {
			t.Fatal(e)
		}
		after, _ := json.Marshal(d)
		if string(before) != string(after) {
			t.Fatal("same choice changed accepted realization")
		}
		if _, e = CompileAuthoredMission(data, reopened, in); e != nil {
			t.Fatal("BEGIN", e)
		}
	}
	token := AuthoredSupportEditFingerprint(d)
	before, _ := json.Marshal(d)
	if e = CommitAuthoredSupportChoice(data, d, AuthoredOption{Formation: 900, Component: 900, Template: 900}, token); e == nil {
		t.Fatal("invalid choice accepted")
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("failed edit mutated accepted document")
	}
	if e = CommitAuthoredSupportChoice(data, d, options[1], token); e != nil {
		t.Fatal(e)
	}
	if e = CommitAuthoredSupportChoice(data, d, options[0], token); e == nil {
		t.Fatal("stale popup owner accepted")
	}
	if _, e = CompileAuthoredMission(data, d, in); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Save(d, "stale"); e == nil {
		t.Fatal("stale SAVE accepted")
	}
}

func TestSupportLegacyConversionPreservesGraph(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	old, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	_ = old
	placements, _ := json.Marshal(d.Placements)
	prior := AuthoredRevision(d)
	if err = CommitAuthoredSupportChoice(data, d, AuthoredOption{Component: -1}, AuthoredSupportEditFingerprint(d)); err != nil {
		t.Fatal(err)
	}
	if d.SupportEdits == nil || d.SupportEdits.PreviousRevision != prior || d.SupportMemory != nil {
		t.Fatal("conversion provenance")
	}
	got, _ := json.Marshal(d.Placements)
	if string(got) != string(placements) {
		t.Fatal("conversion changed graph")
	}
	if _, err = CompileAuthoredMission(data, d, in); err != nil {
		t.Fatal(err)
	}
}
