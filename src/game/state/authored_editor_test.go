package state

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHeadquartersExactSourceMembersAuthorSaveReopen(t *testing.T) {
	data := authoredTestData(t)
	for _, world := range []string{"GULF2.WRL", "GULF3.WRL", "GULF4.WRL", "GULF5.WRL", "EURO6.WRL", "EURO7.WRL", "EURO8.WRL", "ANTC1.WRL", "PHIL1.WRL", "PHIL2.WRL"} {
		t.Run(world, func(t *testing.T) {
			w, err := AuthoredWorld(data, world)
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewAuthoredDocument(data, world, "Headquarters source", 7)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range []struct {
				role       string
				kind, x, y int
			}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
				o, e := AuthoredOptions(data, d, v.role, v.kind)
				if e != nil || len(o) == 0 {
					t.Fatal(e)
				}
				if e = PlaceAuthored(data, d, v.role, v.kind, o[0].Formation, v.x, v.y); e != nil {
					t.Fatal(e)
				}
			}
			opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 10, true)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, o := range opts {
				if o.Name != "Headquarters" {
					continue
				}
				n++
				q, _ := CloneAuthored(d)
				before, _ := json.Marshal(q)
				if err = PlaceAuthoredOption(data, q, "defender", 0, o, 400, 400); err != nil {
					t.Fatal(o.Formation, err)
				}
				p := q.Placements[len(q.Placements)-1]
				rows, e := authoredPlacementComponents(w, p)
				if e != nil || len(rows) != 8 {
					t.Fatal(o.Formation, rows, e)
				}
				//Independent retained Gulf operands: first leader44/kind7, three
				//different actual defensive variants, embedded building137/kind17.
				if w.Theatre == 1 && (w.FRMD[rows[0]*8+1] != 44 || w.FRMD[rows[3]*8+1] != 137 || w.OBJL[p.Template*32+10] != 7) {
					t.Fatal("actual HQ source identities")
				}
				remain0, _ := AuthoredBudget(data, d)
				remain1, e := AuthoredBudget(data, q)
				if e != nil || remain0-remain1 != 8 {
					t.Fatal("eight source component budget", remain0, remain1, e)
				}
				in := MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, PilotName: "HQ source", Seed: 7}
				m, e := CompileAuthoredMission(data, q, in)
				if e != nil {
					t.Fatal(o.Formation, e)
				}
				found := 0
				for _, r := range m.Objects {
					if r.Formation == o.Formation {
						found++
					}
				}
				if found != 8 {
					t.Fatal("actual HQ compiled members", o.Formation, found)
				}
				store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
				rev, e := store.Save(q, "")
				if e != nil {
					t.Fatal(e)
				}
				saved, read, e := store.Load(q.ID)
				if e != nil || rev != read {
					t.Fatal(e)
				}
				frozen, _ := cloneAuthoredRealization(saved.Compiled)
				again, e := CompileAuthoredMission(data, saved, in)
				if e != nil || !reflect.DeepEqual(frozen, saved.Compiled) || again.ObjectHash != m.ObjectHash {
					t.Fatal("unchanged HQ BEGIN", e)
				}
				if err = RemoveAuthoredSource(data, q, p.ID); err != nil {
					t.Fatal(err)
				}
				//An explicit new placement opts into the existing P5 constructor
				//profile; removing it restores every author value except that named
				//admission epoch, not a historical compiler downgrade.
				var expected AuthoredDocument
				_ = json.Unmarshal(before, &expected)
				expected.Compiler = AuthoredCompilerP5
				want, _ := json.Marshal(&expected)
				if got, _ := json.Marshal(q); string(want) != string(got) {
					t.Fatal("HQ delete did not restore detached author values")
				}
			}
			if n < 3 {
				t.Fatal("required source HQ variants", n)
			}
		})
	}
}

func TestChooserFormationMasksAreSourceNotBlanketAircraft(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_AUDIO_ORIGINAL")
	if dir == "" {
		t.Skip("hash-bound original GS fixture required")
	}
	b, e := os.ReadFile(filepath.Join(dir, "GS.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, want := range map[int][]byte{0x3751: {0xc7, 0x86, 0x26, 0xfe, 0x02, 0x00}, 0x3757: {0xc7, 0x86, 0x28, 0xfe, 0x40, 0x00}, 0x3779: {0xc7, 0x86, 0x26, 0xfe, 0x02, 0x10}, 0x3824: {0x23, 0x96, 0x26, 0xfe, 0x23, 0xbe, 0x28, 0xfe}} {
		for n, v := range want {
			if b[0x287c0+ip+n] != v {
				t.Fatal("source mask instruction", ip)
			}
		}
	}
	data := authoredTestData(t)
	for _, w := range data.Worlds {
		d, e := NewAuthoredDocument(data, w.WorldName, "Source chooser", 1)
		if e != nil {
			continue
		}
		for cat := 1; cat <= 10; cat++ {
			opts, _, e := AuthoredEditorOptions(data, d, "defender", 0, cat, true)
			if e != nil {
				t.Fatal(e)
			}
			for _, o := range opts {
				mask := uint32(0x00400002)
				if cat == 10 {
					mask |= 0x1000
				}
				if binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])&mask != 0 {
					t.Fatal(w.WorldName, cat, o.Name, "source-ineligible formation exposed")
				}
			}
		}
	}
}
