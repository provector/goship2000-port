package assets

import (
	"os"
	"reflect"
	"testing"
)

func TestPreparation30UIAndHistorical29RemainSeparate(t *testing.T) {
	old, current := os.Getenv("GS2000_P6_OLD29"), os.Getenv("GS2000_P7_OLD30")
	if old == "" || current == "" {
		t.Skip("explicit hash-bound current30 and historical29 fixtures required")
	}
	a, e := LoadFrontEndAssets(old)
	if e != nil {
		t.Fatal(e)
	}
	b, e := LoadFrontEndAssets(current)
	if e != nil {
		t.Fatal(e)
	}
	if ValidateCurrentFrontEndCapabilities(a) == nil {
		t.Fatal("29 unexpectedly has the new source Support captions")
	}
	// P7's current UI additionally requires the two exact defense constructors
	// and Delete text. The retained P6 reader/content witness remains on30.
	if e = ValidateCurrentFrontEndCapabilities(b); e == nil {
		t.Fatal("historical30 unexpectedly claims Preparation31 capabilities")
	}
	for k, want := range map[string]string{"gps-support-aircraft": "- Aircraft", "gps-support-artillery": "- Artillery : 3"} {
		if a.Cockpit.Literals[k] != "" || b.Cockpit.Literals[k] != want {
			t.Fatal(k, a.Cockpit.Literals[k], b.Cockpit.Literals[k])
		}
	}
	if !reflect.DeepEqual(a.Missions.Generation, b.Missions.Generation) {
		t.Fatal("UI-only upgrade changed authored dependency/generation semantics")
	}
	for _, q := range []struct{ root, want string }{{old, LogicalPreparation29}, {current, LogicalPreparation30}} {
		got, e := LogicalResourceProfile(q.root)
		if e != nil || got != q.want {
			t.Fatal(q, got, e)
		}
	}
}
