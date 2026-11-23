package assets

import (
	"os"
	"testing"
)

// The fixtures are complete, separately generated installations. A one-byte
// change in a consumed model record is the negative control.
func TestRuntimeProfileAcrossOriginalInstallations(t *testing.T) {
	clean, used, changed := os.Getenv("GS2000_TEST_CLEAN_PACKS"), os.Getenv("GS2000_TEST_USED_PACKS"), os.Getenv("GS2000_TEST_CHANGED_PACKS")
	if clean == "" || used == "" || changed == "" {
		t.Skip("requires clean, used, and changed model-table preparations")
	}
	var baseline string
	for _, tc := range []struct {
		dir      string
		released bool
	}{{clean, true}, {used, true}, {changed, false}} {
		profile, err := LogicalResourceProfile(tc.dir)
		if err != nil {
			t.Fatal(err)
		}
		if (profile == LogicalPreparation34) != tc.released {
			t.Fatal("unexpected released identity", profile)
		}
		r, err := newPreparedReader(tc.dir)
		if err != nil {
			t.Fatal(err)
		}
		leaves, err := runtimeLeaves(r)
		r.close()
		if err != nil {
			t.Fatal(err)
		}
		digest, err := runtimeDigest(leaves)
		if err != nil {
			t.Fatal(err)
		}
		if baseline == "" {
			baseline = digest
		}
		if (digest == baseline) != tc.released {
			t.Fatal("unexpected runtime equivalence", digest)
		}
		if !tc.released && profile != digest {
			t.Fatal("changed bytes did not retain their own identity")
		}
		t.Logf("runtime SHA-256 %s; recording identity %s", digest, profile)
	}
}
