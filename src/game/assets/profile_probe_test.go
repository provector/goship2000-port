package assets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// This opt-in diagnostic prints the complete verified canonical inventory
// fingerprint. It does not authorize an unknown profile in production.
func TestProfileProbe(t *testing.T) {
	root := os.Getenv("GS2000_P4_PROFILE_PROBE")
	if root == "" {
		t.Skip("optional full-inventory preparation diagnostic")
	}
	r, e := newPreparedReader(root)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	digest, e := canonicalLogicalResourceProfile(r, &m)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Printf("P4_LOGICAL_PROFILE=%s\n", digest)
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateCurrentFrontEndCapabilities(a); e != nil {
		t.Fatal(e)
	}
	audio, e := LoadAudioAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if audio.GS2Commands[4].Start != 0x8d8 || audio.GS2Commands[5].Start != 0x8d0 {
		t.Fatal("warning admission differs")
	}
	if a.World.Combat.Version != 2 || len(a.World.Combat.DeathHistory) != 8 {
		t.Fatal("history assets missing")
	}
}
