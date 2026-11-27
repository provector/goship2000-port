package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTemplateFreshExistingAndDamaged(t *testing.T) {
	app := t.TempDir()
	dir := filepath.Join(app, "saves")
	source := t.TempDir()
	open := func() *SaveStore {
		t.Helper()
		s, e := OpenManagedCareer(dir, source, app, true)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	s := open()
	initial := s.State()
	p := initial.Slots[0]
	if initial.SelectedPilotID != "" || p == nil || p.Name != `Matt "ProVector" Baker` || p.Status != StatusMIA || p.Rank != RankLieutenantColonel || p.Statistics.MissionCampaignCounter != 44 || p.Squadron != "Retro Works" || !reflect.DeepEqual(p.Statistics.AwardCounts, []int8{1, 1, 1, 2, 2, 3, 2, 1}) {
		t.Fatal("template pilot fields")
	}
	erase := -1
	for i, p := range initial.Slots {
		if p == nil {
			t.Fatal("missing supplied slot")
		}
		if p.Name == "Erase Me" {
			erase = i
		}
	}
	if erase < 0 {
		t.Fatal("missing Erase Me")
	}
	next := s.State()
	newPilot, e := NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	newPilot.Name = "Replacement"
	next.Slots[0].Name = "Existing career"
	next.Slots[erase] = newPilot
	next.SelectedPilotID = newPilot.ID
	if e = s.Commit(next); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s = open()
	if s.State().SelectedPilotID != newPilot.ID || s.State().Slots[0].ID != initial.Slots[0].ID || s.State().Slots[0].Name != "Existing career" {
		t.Fatal("reseeded existing store")
	}
	s.Close()
	if e = os.Remove(filepath.Join(dir, "pilots", playerPack(3))); e != nil {
		t.Fatal(e)
	}
	if broken, e := OpenManagedCareer(dir, source, app, true); e == nil {
		broken.Close()
		t.Fatal("partially deleted store reseeded")
	}
}
func TestTransferredAuthoredAndGeneratedDocuments(t *testing.T) {
	a := missionReadinessAssets(t)
	for _, kind := range []string{"authored", "generated"} {
		t.Run(kind, func(t *testing.T) {
			doc, in := authoredReady(t, a.Generation)
			if kind == "generated" {
				in.MissionType = MissionSingle
				in.Seed = 3
				m, e := GenerateMission(a.Generation, in)
				if e != nil {
					t.Fatal(e)
				}
				if e = PrepareMissionTargets(a, m); e != nil {
					t.Fatal(e)
				}
				doc, e = NewGeneratedDocument(a.Generation, m.GenerationIndex(), m, "Transfer")
				if e != nil {
					t.Fatal(e)
				}
				in.MissionType = MissionSimulated
			}
			before, e := CompileAuthoredMission(a.Generation, doc, in)
			if e != nil {
				t.Fatal(e)
			}
			origin := AuthoredStore{t.TempDir()}
			if _, e = origin.SaveExport(doc); e != nil {
				t.Fatal(e)
			}
			p, err := origin.resolveDocument(doc.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.Remove(p); e != nil {
				t.Fatal(e)
			}
			dest := AuthoredStore{t.TempDir()}
			alias := filepath.Join(dest.Directory, "Custom filename.g2m")
			if e = os.WriteFile(alias, b, 0600); e != nil {
				t.Fatal(e)
			}
			rows, e := dest.List()
			if e != nil || len(rows) != 1 || rows[0].ID != doc.ID {
				t.Fatal(rows, e)
			}
			d, rev, e := dest.Load(doc.ID)
			if e != nil {
				t.Fatal(e)
			}
			actual, e := CompileAuthoredMission(a.Generation, d, in)
			if e != nil || !reflect.DeepEqual(before.Objects, actual.Objects) || !reflect.DeepEqual(before.Paths, actual.Paths) {
				t.Fatal("transferred initial mission changed", e)
			}
			for i, p := range d.Placements {
				if p.Role == "primary" {
					d.Placements[i].X++
					break
				}
			}
			if kind == "authored" {
				d.Conditions.WindSpeed = (d.Conditions.WindSpeed + 1) % 31
			}
			d.Compiled = nil
			edited, e := CompileAuthoredMission(a.Generation, d, in)
			if e != nil {
				t.Fatal(e)
			}
			if reflect.DeepEqual(before.Objectives, edited.Objectives) && before.Conditions.WindSpeed == edited.Conditions.WindSpeed {
				t.Fatal("edit did not reach compiler")
			}
			if _, e = dest.Save(d, rev); e != nil {
				t.Fatal(e)
			}
			d, _, e = (AuthoredStore{dest.Directory}).Load(doc.ID)
			if e != nil {
				t.Fatal(e)
			}
			launched, e := CompileAuthoredMission(a.Generation, d, in)
			if e != nil || (!reflect.DeepEqual(edited.Objectives, launched.Objectives) || edited.Conditions != launched.Conditions) {
				t.Fatal("reopened compiler lost edit", e)
			}
		})
	}
}

func TestOriginalTakesPrecedence(t *testing.T) {
	exe := os.Getenv("GS2000_TEST_ORIGINAL")
	if exe == "" {
		t.Skip("requires supported fixture GS.GS2")
	}
	code, e := os.ReadFile(filepath.Join(exe, "GS.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	original, app := t.TempDir(), t.TempDir()
	if e = os.WriteFile(filepath.Join(original, "gs.gs2"), code, 0600); e != nil {
		t.Fatal(e)
	}
	raw := syntheticLegacyRoster()
	if e = os.WriteFile(filepath.Join(original, "roster.dat"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	s, e := OpenManagedCareer(filepath.Join(app, "saves"), original, app, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.State().Legacy == nil || !reflect.DeepEqual(s.State().Legacy.RawRoster, raw) {
		t.Fatal("original import bypassed")
	}
}
