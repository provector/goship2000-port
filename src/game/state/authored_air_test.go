package state

import (
	"encoding/binary"
	"encoding/csv"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestAllSurveyedIngressChoicesSaveReopenCompile(t *testing.T) {
	data := authoredTestData(t)
	f, err := os.Open(testfixture.Path("editor-air-options.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	worlds := map[string]bool{}
	for _, r := range rows[1:] {
		if r[6] != "01" {
			continue
		}
		count++
		worlds[r[0]] = true
		fi, _ := strconv.Atoi(r[2])
		ci, _ := strconv.Atoi(r[3])
		ti, _ := strconv.Atoi(r[4])
		t.Run(r[0]+"/"+r[2]+"/"+r[3], func(t *testing.T) {
			d, err := NewAuthoredDocument(data, r[0], "R23 ingress", 21)
			if err != nil {
				t.Fatal(err)
			}
			for _, goal := range []struct {
				role       string
				kind, x, y int
			}{{"base", 0, 800, 500}, {"primary", 7, 800, 700}, {"secondary", 3, 900, 700}} {
				opts, e := AuthoredOptions(data, d, goal.role, goal.kind)
				if e != nil || len(opts) == 0 {
					t.Fatal(goal, e)
				}
				if e = PlaceAuthored(data, d, goal.role, goal.kind, opts[0].Formation, goal.x, goal.y); e != nil {
					t.Fatal(e)
				}
			}
			opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			var o AuthoredOption
			found := false
			for _, v := range opts {
				if v.Formation == fi && v.Component == ci && v.Template == ti {
					o, found = v, true
					break
				}
			}
			if !found || o.Unavailable != "" || o.SelectionProfile != AuthoredSelectionProfileR23 {
				t.Fatal("source ingress catalogue", o, found)
			}
			context, err := NewAuthoredEditorContext(data, d.World)
			if err != nil {
				t.Fatal(err)
			}
			before := AuthoredRevision(d)
			p, err := context.Preview(d, "defender", 0, o, 768, 500)
			if err != nil {
				t.Fatal(err)
			}
			if AuthoredRevision(d) != before {
				t.Fatal("preview mutated document/RNG")
			}
			if err = PlaceAuthoredOption(data, d, "defender", 0, o, 768, 500); err != nil {
				t.Fatal(err)
			}
			q := d.Placements[len(d.Placements)-1]
			if p.X != q.X || p.Y != q.Y || q.SelectionProfile != AuthoredSelectionProfileR23 || q.Template != ti || q.Component != ci {
				t.Fatal("preview/press identity", p, q)
			}
			w, err := AuthoredWorld(data, d.World)
			if err != nil {
				t.Fatal(err)
			}
			inputs := MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, PilotName: "R23 Test", Seed: 21}
			m, err := CompileAuthoredMission(data, d, inputs)
			if err != nil {
				t.Fatal(err)
			}
			found = false
			for _, object := range m.Objects {
				if object.Formation == fi && object.Raw[10] == 1 && object.Raw[11] == w.FRMD[ci*8+3] && object.Raw[12] == w.OBJL[ti*32+12] {
					want := MissionPoint{int32(341*q.X-1) + int32(int16(binary.LittleEndian.Uint16(w.FRMD[ci*8+4:]))), int32(-455*q.Y+0x7ffff) + int32(int16(binary.LittleEndian.Uint16(w.FRMD[ci*8+6:])))}
					if object.Point() != want {
						t.Fatal("1771 ingress coordinate", object.Point(), want)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("selected source kind1/component missing from compilation")
			}
			store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
			rev, err := store.Save(d, "")
			if err != nil {
				t.Fatal(err)
			}
			reopened, again, err := store.Load(d.ID)
			if err != nil || rev != again {
				t.Fatal(err)
			}
			loaded, err := CompileAuthoredMission(data, reopened, inputs)
			if err != nil || loaded.ObjectHash != m.ObjectHash || !reflect.DeepEqual(loaded.Objects, m.Objects) {
				t.Fatal("unchanged reopen recompiled or changed realization", err)
			}
		})
	}
	if count != 55 || len(worlds) != 14 {
		t.Fatal("survey denominator changed", count, len(worlds))
	}
}

func TestIngress1771And619CIndependentOperands(t *testing.T) {
	data := authoredTestData(t)
	w, err := AuthoredEditorWorld(data, "EURO7.WRL")
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewAuthoredDocument(data, "EURO7.WRL", "R23 boundary", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Step52's independent descriptor116/345/88/36/585 must compile the raw
	// anchor12275/258112 with no random offset-point draws.
	g, err := newGenericMissionPopulation(w, authoredRNG(1), 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := AuthoredPlacement{Role: "defender", Formation: 116, Component: 345, Template: 88, SelectionProfile: AuthoredSelectionProfileR23, X: 36, Y: 585}
	before := *g.rng.authoredCRT
	point, err := authoredPoint(g, p)
	if err != nil || point != (MissionPoint{12275, 258112}) || *g.rng.authoredCRT != before {
		t.Fatal(point, err)
	}
	// Same cursor in the exact center creates a strict left/right/top/bottom
	// tie; verify by an independent enumeration rather than a label assertion.
	got, err := authoredEnemyEdgeShadow(w, d, 768, 576)
	if err != nil {
		t.Fatal(err)
	}
	best := 9999
	want := [2]int{}
	for n := 1; n <= 62; n++ {
		v := n
		if v < 4 {
			v = 4
		}
		if v > 59 {
			v = 59
		}
		for _, c := range [][2]int{{1, v}, {62, v}, {v, 1}, {v, 62}} {
			cell := int(w.Grid[c[0]+64*c[1]])
			distance := absEntry(32-c[0]) + absEntry(32-c[1])
			if distance < best && cell&128 == 0 && cell < len(w.LAND) && w.LAND[cell]&64 == 0 {
				best = distance
				want = [2]int{c[0]*24 + 12, c[1]*18 + 9}
			}
		}
	}
	if got != want {
		t.Fatal("619C order/ties", got, want)
	}
}

func TestBothSupportedSubmarineChoicesSaveReopenCompile(t *testing.T) {
	data := authoredTestData(t)
	for _, worldName := range []string{"ANTC1.WRL", "ANTC3.WRL"} {
		t.Run(worldName, func(t *testing.T) {
			d, err := NewAuthoredDocument(data, worldName, "R23 submarine", 21)
			if err != nil {
				t.Fatal(err)
			}
			for _, goal := range []struct {
				role       string
				kind, x, y int
			}{{"base", 0, 800, 500}, {"primary", 7, 800, 700}, {"secondary", 3, 900, 700}} {
				opts, e := AuthoredOptions(data, d, goal.role, goal.kind)
				if e != nil || len(opts) == 0 {
					t.Fatal(goal, e)
				}
				if e = PlaceAuthored(data, d, goal.role, goal.kind, opts[0].Formation, goal.x, goal.y); e != nil {
					t.Fatal(e)
				}
			}
			opts, _, err := AuthoredEditorOptions(data, d, "defender", 0, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			var chosen AuthoredOption
			found := false
			for _, o := range opts {
				if o.Component == 567 && o.Template == 82 {
					chosen = o
					found = true
					break
				}
			}
			if !found || chosen.Unavailable != "" || chosen.SelectionProfile != AuthoredSelectionProfileR23 {
				t.Fatal(chosen, found)
			}
			if err = PlaceAuthoredOption(data, d, "defender", 0, chosen, 768, 500); err != nil {
				t.Fatal(err)
			}
			w, err := AuthoredWorld(data, worldName)
			if err != nil {
				t.Fatal(err)
			}
			inputs := MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, PilotName: "R23 Test", Seed: 21}
			mission, err := CompileAuthoredMission(data, d, inputs)
			if err != nil {
				t.Fatal(err)
			}
			found = false
			for _, o := range mission.Objects {
				if o.Formation == chosen.Formation && o.Raw[10] == 0x0a && o.Raw[12] == 219 {
					found = true
				}
			}
			if !found {
				t.Fatal("kind0A/model219 missing")
			}
			store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
			rev, err := store.Save(d, "")
			if err != nil {
				t.Fatal(err)
			}
			reopened, again, err := store.Load(d.ID)
			if err != nil || rev != again {
				t.Fatal(err)
			}
			loaded, err := CompileAuthoredMission(data, reopened, inputs)
			if err != nil || !reflect.DeepEqual(loaded.Objects, mission.Objects) {
				t.Fatal("submarine realization changed", err)
			}
		})
	}
}
