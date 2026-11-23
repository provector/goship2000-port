package assets

import (
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

// Step50's independent extraction predates this decoder and pins all twenty
// original chunk identities, including five different payload banks.
func sourceEnemyBanks(t *testing.T) map[string][]EnemyDefinition {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path("enemy-operands/enemy-immutable-chunks.json"))
	if os.IsNotExist(err) {
		t.Skip("requires retained Step50 immutable source operand fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Worlds []struct {
			World string   `json:"world"`
			Hash  string   `json:"chunk_sha256"`
			Rows  []string `json:"rows"`
		} `json:"worlds"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	banks := map[string][]EnemyDefinition{}
	hashes := map[string]bool{}
	for _, w := range doc.Worlds {
		var payload []byte
		for _, text := range w.Rows {
			row, err := hex.DecodeString(text)
			if err != nil || len(row) != 8 {
				t.Fatal(w.World, err)
			}
			payload = append(payload, row...)
		}
		if Hash(payload) != w.Hash {
			t.Fatal("independent fixture differs", w.World)
		}
		rows, err := decodeEnemyDefinitions(w.World, payload)
		if err != nil {
			t.Fatal(err)
		}
		banks[w.World] = rows
		hashes[w.Hash] = true
	}
	if len(banks) != 20 || len(hashes) != 5 {
		t.Fatal("source coverage", len(banks), len(hashes))
	}
	return banks
}

func TestImmutableEnemyDefinitionsAllWorldsAndBanks(t *testing.T) {
	g := &MissionGenerationData{EnemyDefinitions: sourceEnemyBanks(t)}
	if err := g.ValidateEnemyDefinitions(); err != nil {
		t.Fatal(err)
	}
	for world, source := range g.EnemyDefinitions {
		rows, err := g.EnemyDefinitionsForWorld(world)
		if err != nil || len(rows) != 16 {
			t.Fatal(world, err)
		}
		for i, row := range rows {
			if row != source[i] || row[0] != byte(i) {
				t.Fatal(world, i, row)
			}
		}
		rows[0][2]++
		if source[0][2] == rows[0][2] {
			t.Fatal("launch copy mutated prepared bank", world)
		}
	}
}

func TestEnemyCapabilityRejectsAbsentSwappedTruncatedAndMutated(t *testing.T) {
	g := &MissionGenerationData{EnemyDefinitions: sourceEnemyBanks(t)}
	if err := (&MissionGenerationData{}).ValidateEnemyDefinitions(); err == nil {
		t.Fatal("historical absent capability accepted for new launch")
	}
	if _, err := g.EnemyDefinitionsForWorld("EURO0.WRL"); err == nil {
		t.Fatal("unknown world guessed")
	}
	original := g.EnemyDefinitions["EURO7.WRL"]
	for name, rows := range map[string][]EnemyDefinition{
		"truncated": original[:15], "wrong-world-bank": g.EnemyDefinitions["GULF5.WRL"],
	} {
		g.EnemyDefinitions["EURO7.WRL"] = rows
		if err := g.ValidateEnemyDefinitions(); err == nil {
			t.Fatal(name)
		}
	}
	g.EnemyDefinitions["EURO7.WRL"] = append([]EnemyDefinition(nil), original...)
	g.EnemyDefinitions["EURO7.WRL"][3][2]++
	if _, err := g.EnemyDefinitionsForWorld("EURO7.WRL"); err == nil {
		t.Fatal("transformed/mutated raw bank accepted")
	}
}

func TestHistoricalMetadataDoesNotGainEnemyFields(t *testing.T) {
	raw := []byte(`{"Version":1,"Worlds":null,"Tables":{}}`)
	var d MissionGenerationData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["EnemyDefinitions"]; exists {
		t.Fatal("opening old metadata invented an enemy capability")
	}
}

func TestPreparedEnemyCapability(t *testing.T) {
	root := os.Getenv("GS2000_TEST_R21_ASSETS")
	if root == "" {
		t.Skip("requires declared owned Preparation32 output")
	}
	a, err := LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	// This retained32 fixture proves the R21 ENEM/audio interpretation, not
	// current33 map readiness. Current startup separately upgrades its known
	// bad PHIL1 atlas; do not replace this historical resource expectation.
	if err = a.Missions.Generation.ValidateEnemyDefinitions(); err != nil {
		t.Fatal(err)
	}
	if !a.Audio.SupportsStationaryHostileAudio() {
		t.Fatal("retained R21 hostile audio missing")
	}
	r, err := newPreparedReader(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.read("manifest.json", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = DecodeObject(raw, &m); err != nil || m.PreparationVersion != 32 {
		t.Fatal(err, m.PreparationVersion)
	}
	canonical, err := canonicalLogicalResourceData(r, &m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := canonicalLogicalResourceDataMode(r, &m, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Preparation32 complete profile", Hash(canonical), "decoded", Hash(decoded))
	if Hash(canonical) != LogicalPreparation32 || Hash(decoded) != decodedPreparation32 {
		t.Fatal("complete prepared profile changed")
	}
	if profile, err := LogicalResourceProfile(root); err != nil || profile != LogicalPreparation32 {
		t.Fatal("public profile", profile, err)
	}
}

func TestPreAudioCandidateRemainsReadableButNeedsUpgrade(t *testing.T) {
	root := os.Getenv("GS2000_TEST_R21_CANDIDATE_ASSETS")
	if root == "" {
		t.Skip("requires declared pre-audio29 Preparation32 candidate")
	}
	a, err := LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if profile, err := LogicalResourceProfile(root); err != nil || profile != LogicalPreparation32Candidate {
		t.Fatal("candidate interpretation lost", profile, err)
	}
	if a.Audio.SupportsStationaryHostileAudio() || ValidateCurrentFrontEndCapabilities(a) == nil {
		t.Fatal("missing hostile29 candidate admitted as current capability")
	}
}
