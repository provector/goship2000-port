package menus

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

func TestHistoricalResourceMappings(t *testing.T) {
	for _, tc := range []struct {
		current, recorded, simulation, input string
		want                                 bool
	}{
		{assets.LogicalPreparation26, compatiblePreparation26Resources, replay.AuthoredCompatibility, replay.InputCompatibility, true},
		{assets.LogicalPreparation26, compatiblePreparation25Resources, replay.Compatibility, replay.InputCompatibility, true},
		{assets.LogicalPreparation26, legacyOResources, replay.LegacyCompatibility, replay.LegacyInputCompatibility, true},
		{assets.LogicalPreparation25, legacyOResources, replay.LegacyCompatibility, replay.LegacyInputCompatibility, true},
		{assets.LogicalPreparation24, legacyOResources, replay.LegacyCompatibility, replay.LegacyInputCompatibility, true},
		{assets.LogicalPreparation26, legacyOResources, replay.Compatibility, replay.InputCompatibility, false},
		{assets.LogicalPreparation26, compatiblePreparation25Resources, replay.AuthoredCompatibility, replay.InputCompatibility, false},
		{assets.LogicalPreparation25, compatiblePreparation26Resources, replay.AuthoredCompatibility, replay.InputCompatibility, false},
		{assets.LogicalPreparation26, "unknown-folder-hash", replay.Compatibility, replay.InputCompatibility, false},
		{"unknown-content", compatiblePreparation26Resources, replay.AuthoredCompatibility, replay.InputCompatibility, false},
		{"unknown-content", "unknown-content", replay.Compatibility, replay.InputCompatibility, false},
	} {
		f := &replay.Film{Header: replay.Header{Resources: tc.recorded, Simulation: tc.simulation, InputSchema: tc.input}}
		if got := filmResourceCompatible(f, tc.current); got != tc.want {
			t.Fatalf("mapping %+v got%t", tc, got)
		}
	}
}
func filmEnv(t *testing.T, name string) string {
	t.Helper()
	p := os.Getenv(name)
	if p == "" {
		t.Skip("explicit release fixture " + name + " required")
	}
	return p
}
func TestFilmsAndDocumentsRelocate(t *testing.T) {
	verifyFilmsAndDocumentsRelocate(t, filmEnv(t, "GS2000_P1_FLAT_ASSETS"), filmEnv(t, "GS2000_P1_NESTED_ASSETS"), assets.LogicalPreparation26)
}

// P.3 reuses the strict historical reader checks against the new complete UI
// bundle. The historical Preparation26 control above keeps its exact identity.
func TestHistoricalFilmsAndDocumentsOnPreparation28(t *testing.T) {
	current := os.Getenv("GS2000_P4_RETAINED28")
	if current == "" {
		current = filmEnv(t, "GS2000_TEST_MISSIONS")
	}
	verifyFilmsAndDocumentsRelocate(t, filmEnv(t, "GS2000_P3_FLAT28"), current, assets.LogicalPreparation28)
}

func verifyFilmsAndDocumentsRelocate(t *testing.T, flat, nested, expectedProfile string) {
	paths := []string{filmEnv(t, "GS2000_O2_FULL_FILM"), filmEnv(t, "GS2000_P1_LEGACY_FILM"), filmEnv(t, "GS2000_P1_AUTHORED_FILM")}
	receiptPath, docPath := filmEnv(t, "GS2000_P1_LEGACY_RECEIPT"), filmEnv(t, "GS2000_P1_AUTHORED_DOCUMENT")
	originals := map[string][]byte{}
	for _, p := range append(append([]string{}, paths...), receiptPath, docPath) {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		originals[p] = b
	}
	receipt, e := replay.UnmarshalRecovery(originals[receiptPath])
	if e != nil {
		t.Fatal(e)
	}
	for i, root := range []string{flat, nested} {
		b, e := os.ReadFile(filepath.Join(root, "manifest.json"))
		if e != nil {
			t.Fatal(e)
		}
		var m assets.Manifest
		if e = json.Unmarshal(b, &m); e != nil {
			t.Fatal(e)
		}
		inst, e := assets.ResolveSourceInstallation(&m)
		if e != nil {
			t.Fatal(e)
		}
		isFlat := inst.Root == "" || inst.Root == "."
		if (i == 0) != isFlat {
			t.Fatal("flat/nested roles didnot exercise distinct actual source roots", root, inst.Root)
		}
	}
	var worlds any
	endings := map[int]map[string]string{}
	for _, root := range []string{flat, nested} {
		a, e := assets.LoadFrontEndAssets(root)
		if e != nil {
			t.Fatal(root, e)
		}
		id, e := filmResourceIdentity(a)
		if e != nil || id != expectedProfile {
			t.Fatal("resource profile", root, id, e)
		}
		if worlds == nil {
			worlds = a.Missions.Generation.Worlds
		} else if !reflect.DeepEqual(worlds, a.Missions.Generation.Worlds) {
			t.Fatal("source world/doc dependency changed on relocation")
		}
		// The .g2m dependency hashes MissionWorld's consumed scalar/byte tables, not
		// SourceLocation. Copy and compile through the real retained-document reader.
		dir := t.TempDir()
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(docPath)), originals[docPath], 0600); e != nil {
			t.Fatal(e)
		}
		store := gamestate.AuthoredStore{Directory: dir}
		doc, _, e := store.Load(filepath.Base(docPath)[:32])
		if e != nil {
			t.Fatal(e)
		}
		w, e := gamestate.AuthoredWorld(a.Missions.Generation, doc.World)
		if e != nil {
			t.Fatal(e)
		}
		_, e = gamestate.CompileAuthoredMission(a.Missions.Generation, doc, gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: []gamestate.Theatre{gamestate.TheatreCentralEurope, gamestate.TheatrePersianGulf, gamestate.TheatreAntarctica, gamestate.TheatrePhilippines}[w.Theatre], Rank: gamestate.RankCaptain, PilotName: "Current relocated pilot", Seed: int64(doc.Seed)})
		if e != nil {
			t.Fatal("relocated .g2m BEGIN prerequisites", e)
		}
		for _, scale := range []int{1, 2} {
			cfg := configuration.Defaults()
			cfg.SimulationRenderScale = scale
			cfg.PresentationFPS = 30
			cfg.PresentationMode = "stepped"
			if scale == 2 {
				cfg.PresentationFPS = 60
				cfg.PresentationMode = "smooth"
			}
			for i, path := range paths {
				f, e := replay.Unmarshal(originals[path])
				if e != nil {
					t.Fatal(e)
				}
				var q *FilmPlayback
				if i == 1 {
					bound, digest := portableRecoveryFixture(t, path, receipt)
					q, e = NewRecoveredSource(a, f, digest, bound, cfg)
				} else {
					q, e = NewFilmPlayback(a, f, cfg)
				}
				if e != nil {
					t.Fatal(root, scale, path, e)
				}
				// A verified fresh takeover keeps the isolated runtime and does not gain
				// career/cache/settings/recording capabilities merely by changing assets.
				if i == 2 {
					c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Runtime: cfg, Viewer: NewReplayViewer(q), Screen: ScreenReplay}
					before := q.Preview().FilmCheckpoint()
					if e = c.activateReplayControl(16); e != nil {
						t.Fatal(e)
					}
					if q.Engine.Store != nil || q.Preview().filmRecorder != nil || compareFilmCheckpoint(before, q.Preview().FilmCheckpoint()) != "" {
						t.Fatal("Resume changed state/capability")
					}
					if e = q.AdvancePractice(.1); e != nil {
						t.Fatal(e)
					}
					q, e = NewFilmPlayback(a, f, cfg)
					if e != nil {
						t.Fatal(e)
					}
				}
				for !q.Done {
					if e = q.Step(); e != nil {
						t.Fatal(root, scale, path, q.Cursor, e)
					}
					if q.Cursor%1000 == 0 && !q.Done {
						c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Runtime: cfg, Viewer: NewReplayViewer(q), Screen: ScreenReplay}
						before := q.Preview().FilmCheckpoint()
						c.Viewer.Map = true
						c.Frame(false)
						c.Viewer.Map = false
						c.Viewer.Demo = true
						c.Frame(false)
						if compareFilmCheckpoint(before, q.Preview().FilmCheckpoint()) != "" {
							t.Fatal("viewer presentation mutated gameplay")
						}
					}
				}
				if q.Divergence != "" {
					t.Fatal(q.Divergence)
				}
				end := q.Preview().FilmCheckpoint()
				if old := endings[i]; old != nil && !reflect.DeepEqual(old, end) {
					t.Fatal("root/scale changed ending", path)
				}
				endings[i] = end
				t.Logf("root=%s scale%d film=%s all operations/checkpoints/end matched (%d)", root, scale, filepath.Base(path), q.Cursor)
			}
		}
	}
	for p, b := range originals {
		after, e := os.ReadFile(p)
		if e != nil || !bytes.Equal(b, after) {
			t.Fatal("protected film/receipt/document changed", p, e)
		}
	}
}
