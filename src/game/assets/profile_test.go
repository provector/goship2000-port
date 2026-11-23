package assets

import (
	"os"
	"testing"
)

func TestCurrentAndCandidatePreparation28Capabilities(t *testing.T) {
	old, current := os.Getenv("GS2000_P3_CANDIDATE28"), os.Getenv("GS2000_P3_CURRENT28")
	if old == "" || current == "" {
		t.Skip("hash-bound candidate/current28 fixtures required by release gate")
	}
	for _, v := range []struct {
		root, id string
		ready    bool
	}{{old, LogicalPreparation28Candidate, false}, {current, LogicalPreparation28, false}} {
		h, e := LogicalResourceProfile(v.root)
		if e != nil || h != v.id {
			t.Fatal(h, e)
		}
		a, e := LoadFrontEndAssets(v.root)
		if e != nil {
			t.Fatal(e)
		}
		e = ValidateCurrentFrontEndCapabilities(a)
		if (e == nil) != v.ready {
			t.Fatal("UI capability/profile ownership", v, e)
		}
	}
}
