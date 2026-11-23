package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMissionScreenPreservesControlAndOddChunkBytes(t *testing.T) {
	// ANIM/LABL are runtime control definitions, not office animation objects.
	// An odd serialized label has no generic IFF padding byte.
	control := bytes.Repeat([]byte{7}, 38)
	r := syntheticMenuScreen("ANIM", control)
	s, err := missionScreen(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Chunks) != 2 || s.Chunks[1].RawHex != hex.EncodeToString(control) {
		t.Fatal("control record lost")
	}
	r = syntheticMenuScreen("LABL", []byte{1, 2, 3})
	s, err = missionScreen(r)
	if err != nil || s.Chunks[1].Length != 3 {
		t.Fatal("odd record altered", err)
	}
	r.Payload = r.Payload[:len(r.Payload)-1]
	if _, err = missionScreen(r); err == nil {
		t.Fatal("truncated chunk accepted")
	}
}

func TestMissionMetadataRequiresGenerationNotOnlyImages(t *testing.T) {
	if err := validateMissionMetadata(MissionMetadata{Schema: missionMetadataSchema, Profile: IslandIce469085.Name}); err == nil {
		t.Fatal("metadata without source tables accepted")
	}
}

func TestPreparedMissionAssetsAndFieldPalette(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("set GS2000_TEST_MISSIONS to a newly prepared Step D directory")
	}
	a, err := LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Generation.Worlds) != 16 || len(a.Palettes) != 4 || len(a.Metadata.Poses) != 14 || len(a.AircraftDefinitions) != 8 || len(a.WeaponDefinitions) != 28 {
		t.Fatal("mission dependencies incomplete")
	}
	if a.Fonts[1] == nil || a.Fonts[2] == nil || a.Fonts[3] == nil || a.Fonts[5] == nil {
		t.Fatal("mission fonts missing")
	}
	for _, name := range missionRequiredPictures {
		if len(a.Images[name]) != 64000 {
			t.Fatal("mission picture missing", name)
		}
	}
	if a.Generation.Tables.VisibilityPairs != [2][2]string{{"Good", "Excellent"}, {"Poor", "Limited"}} {
		t.Fatal("authoritative source visibility branch pairing changed")
	}
	// No source input path is read by LoadMissionAssets: this preparation can be
	// moved independently of originals/executables and retains runtime definitions.
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.MissionMetadata)))
	if err != nil {
		t.Fatal(err)
	}
	var doc MissionMetadata
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*MissionMetadata){
		func(d *MissionMetadata) { d.Generation.Tables.Leader = d.Generation.Tables.Leader[:31] },
		func(d *MissionMetadata) { d.Generation.Worlds[0].Grid = d.Generation.Worlds[0].Grid[:4095] },
		func(d *MissionMetadata) { d.Images[0].Asset.Palette.Diagnostic = true },
		func(d *MissionMetadata) { delete(d.Presentation.Screens, "REALITY.SCN") },
		func(d *MissionMetadata) { d.Palettes[0].Components.Size = 767 },
		func(d *MissionMetadata) { d.Fonts[0].Outputs = d.Fonts[0].Outputs[:len(d.Fonts[0].Outputs)-1] },
	} {
		var broken MissionMetadata
		if err = json.Unmarshal(raw, &broken); err != nil {
			t.Fatal(err)
		}
		mutate(&broken)
		if err = validateMissionMetadata(broken); err == nil {
			t.Fatal("malformed source dependency accepted")
		}
	}
	s := a.Metadata.Screens["REALITY.SCN"]
	found := false
	for _, c := range s.Chunks {
		if c.Tag == "ANIM" && len(c.Payload) == 38 && c.Payload[0] == 0 {
			if c.Payload[25] != 3 {
				t.Fatal("CP/G choice count")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("REALITY raw control data missing")
	}
}
