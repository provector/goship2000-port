package state

import (
	"encoding/binary"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Current compiler integration has an explicit fixture owner. A relocation of
// source/prepared files must not silently select an older research realization.
// The release verifier requires this fixture and rejects a skipped mandatory test.
func authoredTestData(t *testing.T) *assetdata.MissionGenerationData {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_EDITOR")
	if dir == "" {
		dir = os.Getenv("GS2000_TEST_MISSIONS")
	}
	if dir == "" {
		t.Skip("authored compiler prerequisite: set GS2000_TEST_EDITOR to a current prepared bundle")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatalf("authored compiler fixture %q: %v", dir, err)
	}
	if a.Generation == nil || len(a.Generation.Tables.EditorModelCosts) != 230 || len(a.Generation.Tables.EditorObjectiveVerbs) != 8 {
		t.Fatalf("authored compiler fixture %q lacks current editor tables", dir)
	}
	t.Logf("authored compiler fixture: %s", dir)
	return a.Generation
}

func authoredReady(t *testing.T, data *assetdata.MissionGenerationData) (*AuthoredDocument, MissionInputs) {
	t.Helper()
	w := &data.Worlds[0]
	d, e := NewAuthoredDocument(data, w.WorldName, "Pickup test", 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, role := range []struct {
		r       string
		k, x, y int
	}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
		opts, e := AuthoredOptions(data, d, role.r, role.k)
		if e != nil || len(opts) == 0 {
			t.Fatal(role, e, len(opts))
		}
		if e = PlaceAuthored(data, d, role.r, role.k, opts[0].Formation, role.x, role.y); e != nil {
			t.Fatal(role, e)
		}
	}
	return d, MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, PilotName: "Editor source test", Seed: 1}
}
func TestAuthoredSourceKindsAndCRT(t *testing.T) {
	want := []int{1, 6, 3, 5, 3, 5, 7, 4}
	if !reflect.DeepEqual(authoredKinds[:], want) {
		t.Fatal(authoredKinds)
	}
	r := authoredRNG(1)
	if r.r0(1, "none") != 0 || *r.authoredCRT != 1 {
		t.Fatal("bound1 consumed draw")
	}
	if got := r.r0(100, "hand"); got != 0 || *r.authoredCRT != 2745024 {
		t.Fatal(got, *r.authoredCRT)
	}
	if got := r.r0(100, "hand2"); got != 56 || *r.authoredCRT != 3357800067 {
		t.Fatal(got, *r.authoredCRT)
	}
	normal := newMissionRNG(MissionInputs{Seed: 1})
	if normal.authoredCRT != nil {
		t.Fatal("normal generation policy changed")
	}
}

func TestAuthoredBaseAndFARPSourceCarriers(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	w, e := AuthoredWorld(data, d.World)
	if e != nil || w.WorldName != "EURO6.WRL" {
		t.Fatal("independent source case requires EURO6", e)
	}
	// Original FRMD records: base ID1 starts3 and has7 components; FARP
	// ID3 starts16 and has5. A191f7 maps counts to4/3, not visits or a constant.
	for _, c := range []struct {
		role                    string
		formation, start, count int
		models                  []byte
	}{{"base", 4, 3, 7, []byte{131, 88, 130, 46, 46, 121, 136}}, {"FARP", 19, 16, 5, []byte{133, 130, 46, 132, 136}}} {
		for i, model := range c.models {
			if w.FRMD[(c.start+i)*8+1] != model || w.FRMD[(c.start+i)*8+2] != 1 {
				t.Fatal("source component operands", c.role, i)
			}
		}
		// Preserve this P2 carrier witness under its recorded descriptor
		// interpretation. P3's live Base menu no longer exposes sourcefi4.
		p, err := authoredPlacement(w, c.formation, c.role, 0)
		if err != nil {
			t.Fatal(err)
		}
		p.SelectionProfile = AuthoredSelectionProfileP2
		p.X, p.Y = 800, 500
		f := w.FMHD[c.formation*39:]
		if binary.LittleEndian.Uint32(f[35:])&0x1000 != 0 {
			p.X = int(int32(binary.LittleEndian.Uint32(f[27:])))/341 - 1
			p.Y = int(int32(binary.LittleEndian.Uint32(f[31:])))/-455 + 1151
		}
		for i, q := range d.Placements {
			if q.Role == c.role {
				d.Placements = append(d.Placements[:i], d.Placements[i+1:]...)
				break
			}
		}
		d.Placements = append(d.Placements, p)
	}
	m, e := CompileAuthoredMission(data, d, in)
	if e != nil {
		t.Fatal(e)
	}
	for i, expected := range []uint16{4, 3} {
		r := m.Targets.Records[i]
		if binary.LittleEndian.Uint16(r[50:]) != expected || binary.LittleEndian.Uint16(r[32:]) != 0x1abe {
			t.Fatal("source base/FARP carrier", i, r)
		}
	}
	// Exercise the actual service reader with the compiled TARGET carrier.
	// It tests present/nonzero and signed losses<allowance; it does not
	// interpret this opaque word as a primary/secondary dispatch kind.
	for _, losses := range []int16{2, 3} {
		i, _, systems := fARPFixture()
		i.Targets = m.Targets
		f := targetServicePoint(m.Targets.Records[1])
		i.PlayerFlight.Pose.H0, i.PlayerFlight.Pose.H1 = uint32(f.X), uint32(f.Y)
		i.Sortie = &SortieState{FARPLosses: losses}
		got, err := i.AdvanceService(m, systems)
		if err != nil || got != (losses == 2) || i.Sortie.FARPLosses != losses || i.Sortie.Pending != "" || i.Sortie.Status&0x80 != 0 {
			t.Fatal("compiled FARP source service", losses, got, err, i.Sortie)
		}
	}
	base, _, systems := sortieFixture()
	base.Targets = m.Targets
	p := targetServicePoint(m.Targets.Records[0])
	base.PlayerFlight.Pose.H0, base.PlayerFlight.Pose.H1 = uint32(p.X), uint32(p.Y)
	if got, err := base.AdvanceService(m, systems); err != nil || !got || base.Sortie.Status != 0x80 || base.Sortie.Pending != "base" {
		t.Fatal("compiled base source service", got, err, base.Sortie)
	}
}
func TestAuthoredCompilerStoreAndTamper(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	budget, e := AuthoredBudget(data, d)
	if e != nil || budget <= 0 || budget >= 71 {
		t.Fatal(budget, e)
	}
	m, e := CompileAuthoredMission(data, d, in)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Objects) != 80 || m.Inputs.MissionType != MissionSimulated {
		t.Fatal("incomplete compiled owner")
	}
	for _, role := range []string{"primary", "secondary"} {
		o := m.Objectives[role]
		if o.ObjectIndex < 9 || o.Count <= 0 {
			t.Fatal(o)
		}
	}
	if m.Objectives["primary"].RawKind != 4 || m.Objectives["primary"].DisplayKind != 2 || m.Targets.Records[2][32] != 2 || m.Objectives["secondary"].DisplayKind != 1 {
		t.Fatal("raw/mapped domains", m.Objectives)
	}
	if m.Targets.Records[4][0] != 0x20 || m.Targets.Records[4][1] != 0 {
		t.Fatal("inactive TARGET initializer")
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "missions")}
	rev, e := store.Save(d, "")
	if e != nil {
		t.Fatal(e)
	}
	loaded, r2, e := store.Load(d.ID)
	if e != nil || rev != r2 {
		t.Fatal(e, rev, r2)
	}
	again, e := CompileAuthoredMission(data, loaded, in)
	if e != nil || again.ObjectHash != m.ObjectHash || !reflect.DeepEqual(again.Targets, m.Targets) {
		t.Fatal("cached source compile", e)
	}
	again.Objects[9].Raw[12] ^= 1
	if d.Compiled.Objects[9].Raw[12] == again.Objects[9].Raw[12] {
		t.Fatal("launch aliases document")
	}
	loaded.Compiled.Targets.Records[2][32] = 0
	if _, e = CompileAuthoredMission(data, loaded, in); e == nil {
		t.Fatal("tampered completion accepted")
	}
	if _, e = store.Save(d, "wrong"); e == nil {
		t.Fatal("external revision ignored")
	}
	raw, err := os.ReadFile(filepath.Join(store.Directory, d.Title+".g2m"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PilotID") {
		t.Fatal("old pilot stored")
	}
	if e = store.Delete(d.ID, rev); e != nil {
		t.Fatal(e)
	}
	entries, e := store.List()
	if e != nil || len(entries) != 0 {
		t.Fatal(e, entries)
	}
}
func TestAuthoredInvalidAndCancelledEdits(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	before := AuthoredRevision(d)
	if e := PlaceAuthored(data, d, "primary", 7, -1, 0, 0); e == nil || AuthoredRevision(d) != before {
		t.Fatal("failed edit mutated document")
	}
	if e := PlaceAuthored(data, d, "primary", 2, 0, 600, 600); e == nil {
		t.Fatal("unclosed intercept enabled")
	}
	if _, e := CompileAuthoredMission(data, d, MissionInputs{MissionType: MissionFlight, Theatre: in.Theatre, Rank: in.Rank}); e == nil {
		t.Fatal("authored flight relabelled")
	}
	for _, p := range d.Placements {
		if p.Builtin {
			if e := RemoveAuthored(d, p.ID); e == nil {
				t.Fatal("built-in deleted")
			}
			break
		}
	}
	d.Placements = nil
	if _, e := CompileAuthoredMission(data, d, in); e == nil {
		t.Fatal("invalid blank source state compiled")
	}
}
func TestAuthoredParserBoundsAndSourcePredicates(t *testing.T) {
	dir := t.TempDir()
	s := AuthoredStore{Directory: dir}
	id := strings.Repeat("a", 32)
	if _, _, e := s.Load("../pilot"); e == nil {
		t.Fatal("path traversal")
	}
	if e := os.WriteFile(filepath.Join(dir, id+".g2m"), []byte(`{"format":"unexpected"}`), 0600); e != nil {
		t.Fatal(e)
	}
	entries, e := s.List()
	if e != nil || len(entries) != 1 || entries[0].Error == "" {
		t.Fatal("one corrupt file makes whole browser inaccessible", entries, e)
	}
	if _, _, e = s.Load(id); e == nil {
		t.Fatal("invalid JSON accepted")
	}
	if e := os.WriteFile(filepath.Join(dir, id+".g2m"), make([]byte, AuthoredMaxBytes+1), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Load(id); e == nil {
		t.Fatal("oversize accepted")
	}
	if authoredKinds[7] != 4 {
		t.Fatal("source eight menu table")
	}
	var r [62]byte
	r[0] = 32
	binary.LittleEndian.PutUint16(r[32:], 2)
	if r[0] != 32 || r[1] != 0 || r[32] != 2 {
		t.Fatal("source hand record")
	}
}

func TestAuthoredExportInitialTargetsAndImmutableOwners(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	m, e := CompileAuthoredMission(data, d, in)
	if e != nil {
		t.Fatal(e)
	}
	x, e := NewAuthoredFromMission(data, m, "Exported pickup", 22)
	if e != nil {
		t.Fatal(e)
	}
	if x.ID == d.ID || x.Compiled == nil || !x.Compiled.Exported {
		t.Fatal("Export did not create independent document")
	}
	again, e := CompileAuthoredMission(data, x, in)
	if e != nil || again.ObjectHash != m.ObjectHash {
		t.Fatal("unchanged Export layout", e)
	}
	again.Targets.Records[2][32] = 0
	if m.Targets.Records[2][32] == 0 || x.Compiled.Targets.Records[2][32] == 0 {
		t.Fatal("Export aliases source targets")
	}
	x.Compiled.Targets.Records[2][32] = 0
	if _, e = CompileAuthoredMission(data, x, in); e == nil {
		t.Fatal("Export pre-completed goal accepted")
	}
	m.Inputs.MissionType = MissionSingle
	if _, e = NewAuthoredFromMission(data, m, "Invalid source", 1); e == nil {
		t.Fatal("Single padded to Simulation")
	}
}
func TestAuthoredCheckedJSONBeforeAllocation(t *testing.T) {
	for _, b := range []string{`{"id":"a","id":"b"}`, `{"placements":[` + strings.Repeat(`{},`, 80) + `{}]}`, `{"compiled":{"targets":{"records":[[` + strings.Repeat(`0,`, 62) + `0]]}}}`} {
		if e := authoredJSONBounds([]byte(b)); e == nil {
			t.Fatal("unbounded/duplicate fixed-array input accepted")
		}
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if e := os.Mkdir(real, 0700); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(dir, "link")
	if e := os.Symlink(real, link); e != nil {
		t.Skip("host cannot create test symlink")
	}
	if unlock, e := (AuthoredStore{Directory: link}).lock(); e == nil {
		unlock()
		t.Fatal("symbolic mission store followed")
	}
}
func TestAuthoredWorldAdmission(t *testing.T) {
	data := authoredTestData(t)
	failures := []string{}
	for _, w := range data.Worlds {
		d, e := NewAuthoredDocument(data, w.WorldName, "World test", 5)
		if w.WorldName == "EUROX.WRL" || w.WorldName == "GULFX.WRL" {
			if e == nil {
				t.Fatal("unadmitted world admitted", w.WorldName)
			}
			continue
		}
		if e != nil {
			failures = append(failures, w.WorldName+":"+e.Error())
			continue
		}
		for _, r := range []struct {
			role string
			kind int
		}{{"base", 0}, {"primary", 7}, {"secondary", 3}} {
			opts, e := AuthoredOptions(data, d, r.role, r.kind)
			if e != nil || len(opts) == 0 {
				failures = append(failures, w.WorldName+" options:"+r.role)
				break
			}
			if e = PlaceAuthored(data, d, r.role, r.kind, opts[0].Formation, 700, 700); e != nil {
				failures = append(failures, w.WorldName+" place:"+r.role+":"+e.Error())
				break
			}
		}
		if len(d.Placements) > 0 {
			if _, e = CompileAuthoredMission(data, d, MissionInputs{MissionType: MissionSimulated, Theatre: saveTheatres[w.Theatre], Rank: RankCaptain, Seed: 5}); e != nil {
				failures = append(failures, w.WorldName+" compile:"+e.Error())
			}
		}
	}
	if len(failures) > 0 {
		t.Fatal("source admission differences", failures)
	}
}

func TestAuthoredCleanFlightExport(t *testing.T) {
	data := authoredTestData(t)
	labels := &assetdata.OutfitObjectiveText{RecordFormat: "%02d/%02d %s %Fs", Verbs: data.Tables.EditorObjectiveVerbs, Abbreviations: data.Tables.EditorObjectiveAbbreviations}
	a := &assetdata.MissionAssets{Preflight: &assetdata.PreflightAssets{ObjectiveSummary: labels}}
	var reasons []string
	for seed := int64(1); seed <= 128; seed++ {
		m, e := GenerateMission(data, MissionInputs{MissionType: MissionFlight, Theatre: TheatrePersianGulf, Rank: RankCaptain, PilotName: "Clean export test", Seed: seed})
		if e != nil {
			continue
		}
		if e = PrepareMissionTargets(a, m); e != nil {
			t.Fatal(e)
		}
		doc, e := NewAuthoredFromMission(data, m, "Clean Flight", uint32(seed))
		if e != nil {
			if len(reasons) < 4 {
				reasons = append(reasons, e.Error())
			}
			continue
		}
		again, e := CompileAuthoredMission(data, doc, MissionInputs{MissionType: MissionSimulated, Theatre: TheatrePersianGulf, Rank: RankCaptain, PilotName: "Current pilot", Seed: seed})
		if e != nil {
			t.Fatal(e)
		}
		if again.Inputs.PilotName != "Current pilot" || again.ObjectHash != m.ObjectHash {
			t.Fatal("Export rebound wrong pilot/layout")
		}
		t.Logf("clean real Flight seed%d world%s primaryRaw%d secondaryRaw%d preserves80rows; no supplied result", seed, m.World, m.Objectives["primary"].RawKind, m.Objectives["secondary"].RawKind)
		return
	}
	t.Fatal("no admitted clean Flight fixture in bounded128seeds", reasons)
}

func TestAuthoredCornerAndCategoryDomains(t *testing.T) {
	data := authoredTestData(t)
	for _, c := range []struct {
		id   byte
		want int
	}{{54, 2}, {34, 4}, {43, 5}, {55, 8}, {22, 10}} {
		if got := authoredCategory(data.Tables, c.id); got != c.want {
			t.Fatal("OBJTYPES category domain", c, got)
		}
	}
	d, _ := authoredReady(t, data)
	opts, e := AuthoredOptions(data, d, "primary", 7)
	if e != nil || len(opts) == 0 {
		t.Fatal(e)
	}
	if e = PlaceAuthored(data, d, "primary", 7, opts[0].Formation, -10, -9); e == nil {
		t.Fatal("outside overview admitted")
	}
	if e = PlaceAuthored(data, d, "primary", 7, opts[0].Formation, -9, -9); e != nil {
		t.Fatal("admitted overview corner rejected before signed cell division", e)
	}
	for _, p := range d.Placements {
		if p.Role == "primary" && (p.X < 0 || p.Y < 0) {
			t.Fatal("negative cursor persisted as accepted placement")
		}
	}
}

func TestAuthoredConsistentDigestDoesNotAuthorizeScoreOrCompletedGoal(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	m, e := CompileAuthoredMission(data, d, in)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"score", "target", "route"} {
		t.Run(bad, func(t *testing.T) {
			x, e := NewAuthoredFromMission(data, m, "Corrupt carrier", 2)
			if e != nil {
				t.Fatal(e)
			}
			switch bad {
			case "score":
				x.Initial.Layout.Objects[9].Raw[29] ^= 1
			case "target":
				x.Initial.Layout.Targets.Records[2][32] = 0
			case "route":
				x.Initial.Layout.Objects[9].Raw[0] |= 2
				binary.LittleEndian.PutUint32(x.Initial.Layout.Objects[9].Raw[6:], 999)
			}
			x.Initial.Layout.ObjectHash = authoredObjectHash(x.Initial.Layout.Objects)
			x.Initial.Digest = authoredDigest(x.Initial.Layout)
			x.Compiled = nil
			if _, e = CompileAuthoredMission(data, x, in); e == nil {
				t.Fatal("consistent JSON checksums accepted unsupported active owner", bad)
			}
		})
	}
}
func TestAuthoredLockOwnership(t *testing.T) {
	app := t.TempDir()
	dir := filepath.Join(app, "saves", "missions")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, ".authored-store.lock")
	if e := os.WriteFile(path, []byte(`{"pid":1}`), 0600); e != nil {
		t.Fatal(e)
	}
	if owner, e := configuration.BeginOwnership(app, "installed", "", false); e == nil {
		owner.Close()
		t.Fatal("unrecognized lock removed")
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal("unknown lock lost")
	}
}

func TestEditorSaveUsesTitleAndAdoptsPreviousFile(t *testing.T) {
	data := authoredTestData(t)
	d, in := authoredReady(t, data)
	if _, err := CompileAuthoredMission(data, d, in); err != nil {
		t.Fatal(err)
	}
	store := AuthoredStore{Directory: filepath.Join(t.TempDir(), "Łódź missions")}
	title := func(s string) { d.Title = s; d.Compiled.Revision = AuthoredRevision(d) }
	title("Arctic Fleet")
	rev, err := store.Save(d, "")
	if err != nil {
		t.Fatal(err)
	}
	path := func(name string) string { return filepath.Join(store.Directory, name+".g2m") }
	if err = os.Rename(path(d.Title), path(d.ID)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Load(d.ID); err != nil {
		t.Fatal(err)
	}
	save := func() {
		t.Helper()
		var e error
		rev, e = store.Save(d, rev)
		if e != nil {
			t.Fatal(e)
		}
	}
	save()
	save()
	if _, e := os.Stat(path(d.ID)); !os.IsNotExist(e) {
		t.Fatal("opaque name survived", e)
	}
	other := []byte("unrelated")
	if e := os.WriteFile(path("NORTH"), other, 0600); e != nil {
		t.Fatal(e)
	}
	title("North")
	save()
	if _, e := os.Stat(path("North (2)")); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(path("NORTH")); string(b) != string(other) {
		t.Fatal("overwrote other document")
	}
	title("CON")
	save()
	title("con")
	save()
	if _, e := os.Stat(path("_con")); e != nil {
		t.Fatal(e)
	}
	title("Ground/Sea")
	save()
	if _, e := os.Stat(path("Ground_Sea")); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"", "Łódź"} {
		title(bad)
		if _, e := store.Save(d, rev); e == nil {
			t.Fatal("invalid portable title accepted", bad)
		}
	}
	loaded, actualRev, e := store.Load(d.ID)
	if e != nil || rev != actualRev || loaded.Title != "Ground/Sea" {
		t.Fatal("lost last good mission", actualRev, e)
	}
	if _, e = CompileAuthoredMission(data, loaded, in); e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS == "windows" {
		f, err := os.Open(path("Ground_Sea"))
		if err != nil {
			t.Fatal(err)
		}
		title("Renamed")
		_, saveErr := store.Save(d, rev)
		closeErr := f.Close()
		if saveErr == nil || closeErr != nil {
			t.Fatal("open-handle rename", saveErr, closeErr)
		}
		previous, unchanged, err := store.Load(d.ID)
		if err != nil || unchanged != rev || previous.Title != "Ground/Sea" {
			t.Fatal("open-handle failure lost previous generation", err)
		}
	}
	// Save As gives the copy its own identity even when the title is unchanged.
	duplicate, err := CloneAuthored(loaded)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.ID, err = authoredID()
	if err != nil {
		t.Fatal(err)
	}
	duplicate.Compiled.Revision = AuthoredRevision(duplicate)
	if _, err = store.Save(duplicate, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path("Ground_Sea (2)")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{loaded.ID, duplicate.ID} {
		doc, _, err := store.Load(id)
		if err != nil || doc.Title != "Ground/Sea" {
			t.Fatal(id, err)
		}
	}

}
