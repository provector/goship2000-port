package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArtilleryCandidateUsesActualSignedLowWordArguments(t *testing.T) {
	// Source1ABF:2033/2037 pushes ACA0/A27C.065C:238A scales only current
	// position, passing those signed words to0D2E. These are not edge locals.
	origin := MissionPoint{198608, 266144}
	coarse := missionCell(origin, 32768)
	actual := [2]int{int(int16(origin.X)), int(int16(origin.Y))}
	if actual == [2]int{5, 0} || missionMetric(coarse, actual) <= 8 {
		t.Fatal(actual, coarse)
	}
	if missionMetric(coarse, [2]int{5, 0}) > 8 {
		t.Fatal("negative control no longer discriminates wrong edge operand")
	}
}
func TestArtilleryCopiedDocumentFirstSaveResaveAndOldInterpretation(t *testing.T) {
	data := authoredTestData(t)
	path := os.Getenv("GS2000_P5_AUTHORED_DOC")
	if path == "" {
		t.Skip("copied user document prerequisite GS2000_P5_AUTHORED_DOC")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var initial AuthoredDocument
	if err = json.Unmarshal(raw, &initial); err != nil {
		t.Fatal(err)
	}
	d, err := CloneAuthored(&initial)
	if err != nil {
		t.Fatal(err)
	}
	options, err := AuthoredSupportOptions(data, d)
	if err != nil {
		t.Fatal(err)
	}
	in := MissionInputs{MissionType: MissionSimulated, Theatre: TheatreCentralEurope, Rank: RankCaptain, PilotName: "P5 source regression", Seed: int64(d.Seed)}
	n := 0
	for _, o := range options {
		if o.Cost != 3 {
			continue
		}
		n++
		q, _ := CloneAuthored(d)
		if err = SetAuthoredSupport(data, q, o); err != nil {
			t.Fatal(o.Name, err)
		}
		if q.Compiler != AuthoredCompilerSupportP5 {
			t.Fatal("new selection not versioned")
		}
		if _, err = CompileAuthoredMission(data, q, in); err != nil {
			t.Fatal(o.Name, err)
		}
		first, _ := cloneAuthoredRealization(q.Compiled)
		if _, err = CompileAuthoredMission(data, q, in); err != nil || !reflect.DeepEqual(first, q.Compiled) {
			t.Fatal("unchanged BEGIN recompiled", err)
		}
		store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
		rev, err := store.Save(q, "")
		if err != nil {
			t.Fatal(err)
		}
		loaded, _, err := store.Load(q.ID)
		if err != nil {
			t.Fatal(err)
		}
		loaded.Title = "P5 edited"
		loaded.Compiled = nil
		if _, err = CompileAuthoredMission(data, loaded, in); err != nil {
			t.Fatal(o.Name, "resave", err)
		}
		if _, err = store.Save(loaded, rev); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s first-save/edit-resave:80 rows Support%v sourceprofile%s", o.Name, loaded.Compiled.Support.Rows, loaded.Compiler)
	}
	if n != 2 {
		t.Fatal("expected actual M109/Rockets alternatives", n)
	}
}
