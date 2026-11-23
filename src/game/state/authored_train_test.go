package state

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTrainTerminalCopiedBeforeIndexIncrement(t *testing.T) {
	root := os.Getenv("GS2000_TEST_AUDIO_ORIGINAL")
	if root == "" {
		t.Skip("source-byte fixture: GS2000_TEST_AUDIO_ORIGINAL required")
	}
	b, err := os.ReadFile(filepath.Join(root, "GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x8b, 0x76, 0xfc, 0xff, 0x46, 0xfc, 0x03, 0x76, 0xfe, 0x8b, 0xc6, 0xc1, 0xe6, 0x03, 0x03, 0xf0, 0xc4, 0x1e, 0x58, 0xb8, 0x26, 0x80, 0x78, 0x08, 0x40}
	if !bytes.Equal(b[0x287c0+0x658d:0x287c0+0x658d+len(want)], want) {
		t.Fatal("source pre-increment index operands differ")
	}
	// Independent literal loop: save old index, increment counter, test copied
	// source row. A test of the next row is the superseded P5 interpretation.
	flags := []byte{8, 0, 0x40}
	copied := []byte{}
	for n := 0; ; {
		copied = append(copied, flags[n])
		old := n
		n++
		if flags[old] == 0x40 {
			break
		}
	}
	if !bytes.Equal(copied, flags) {
		t.Fatal(copied)
	}
}

func TestTrainActualPresetAuthorSaveReopenCompile(t *testing.T) {
	data := authoredTestData(t)
	w, err := AuthoredWorld(data, "EURO7.WRL")
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewAuthoredDocument(data, w.WorldName, "Train source", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		role       string
		kind, x, y int
	}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
		opts, e := AuthoredOptions(data, d, q.role, q.kind)
		if e != nil || len(opts) == 0 {
			t.Fatal(q, e)
		}
		if e = PlaceAuthored(data, d, q.role, q.kind, opts[0].Formation, q.x, q.y); e != nil {
			t.Fatal(q, e)
		}
	}
	paths, err := authoredTrainPaths(w)
	if err != nil {
		t.Fatal(err)
	}
	first, second := authoredTrainAnchor(paths[0][0].Point), authoredTrainAnchor(paths[1][0].Point)
	for _, q := range []struct{ at, want [2]int }{{first, first}, {second, second}} {
		got, e := authoredTrainShadow(w, q.at[0], q.at[1])
		if e != nil || got != q.want {
			t.Fatal(q, got, e)
		}
	}
	options, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var train *AuthoredOption
	for i := range options {
		o := &options[i]
		if binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])&0x200 != 0 {
			train = o
			break
		}
	}
	if train == nil || train.Unavailable != "" {
		t.Fatal("actual source Train option unavailable", train)
	}
	if err = PlaceAuthoredOption(data, d, "defender", 0, *train, first[0], first[1]); err != nil {
		t.Fatal(err)
	}
	p := d.Placements[len(d.Placements)-1]
	if p.TrainProfile != AuthoredTrainProfile || p.X != first[0] || p.Y != first[1] {
		t.Fatal("source shadow/profile", p)
	}
	if err = PlaceAuthored(data, d, "primary", 2, p.Formation, p.X, p.Y); err != nil {
		t.Fatal(err)
	}
	in := MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, PilotName: "Train source", Seed: 1}
	m, err := CompileAuthoredMission(data, d, in)
	if err != nil {
		t.Fatal(err)
	}
	goal := m.Objectives["primary"]
	if m.Objects[goal.ObjectIndex].Raw[10] != 0x40 || len(m.Paths) != 2+len(paths[0]) {
		t.Fatal("real source header/span", goal, len(m.Paths), len(paths[0]))
	}
	for i, q := range paths[0] {
		got := m.Paths[i+2]
		if got.Point != q.Point || got.Flag != q.Flag {
			t.Fatalf("source row%d differs: %+v %+v", i, got, q)
		}
	}
	if m.Paths[len(m.Paths)-1].Flag != 0x40 || goal.Endpoint == nil || *goal.Endpoint != paths[0][len(paths[0])-1].Point {
		t.Fatal("terminal/target endpoint", goal)
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
	if _, err = store.Save(d, ""); err != nil {
		t.Fatal(err)
	}
	reopened, _, err := store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := cloneAuthoredRealization(reopened.Compiled)
	again, err := CompileAuthoredMission(data, reopened, in)
	if err != nil || !reflect.DeepEqual(frozen, reopened.Compiled) || !reflect.DeepEqual(m.Paths, again.Paths) {
		t.Fatal("unchanged BEGIN recompiled", err)
	}
	bad, _ := CloneAuthored(d)
	for i := range bad.Placements {
		if bad.Placements[i].TrainProfile != "" {
			bad.Placements[i].TrainProfile = "unknown"
		}
	}
	if ValidateAuthored(data, bad) == nil {
		t.Fatal("unknown constructor profile admitted")
	}
	t.Logf("actual EURO7 Train fi%d components/header%d PATH%d includes source terminal40; save/reopen/reused realization", p.Formation, goal.ObjectIndex, len(m.Paths))
}
