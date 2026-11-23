package assets

import (
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFlightFeedbackEmbeddedTables(t *testing.T) {
	path := testfixture.Path("installation/GS2.GS2")
	if root := os.Getenv("GS2000_TEST_MISSIONS"); root != "" {
		path = filepath.Join(root, "world/raw/GS2.GS2")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if Hash(b) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported independent source")
	}
	a := &cockpit.Assets{}
	if e = loadFlightFeedbackTables(b, a); e != nil {
		t.Fatal(e)
	}
	if len(a.TargetDossiers) != 230 {
		t.Fatal(len(a.TargetDossiers))
	}
	want := [4]string{"Fuel Tank", "Fuel Tank found at friendly helicopter bases and FARPs. No threat.", "None", "None"}
	if !reflect.DeepEqual(a.TargetDossiers[132], want) {
		t.Fatal(a.TargetDossiers[132])
	}
	if a.TargetDossiers[1][0] != "Small House" || a.TargetDossiers[1][2] != "M261s, Cannon and Minipod" || a.TargetDossiers[1][3] != "Sidearm, Stinger, Sidewinder and Penguin" {
		t.Fatal(a.TargetDossiers[1])
	}
	if a.MemberCaptionLiterals["formed"] != "[ FORMED UP ]" || a.MemberCaptionLiterals["fly"] != "[ FLY TO ]" || a.MemberCaptionLiterals["out"] != "[ OUT OF ACTION ]" {
		t.Fatal(a.MemberCaptionLiterals)
	}
	bad := append([]byte(nil), b...)
	bad[0x2f7d0+0x46a3] = 0xff
	bad[0x2f7d0+0x46a4] = 0xff
	if loadFlightFeedbackTables(bad, &cockpit.Assets{}) == nil {
		t.Fatal("out-of-extent string accepted")
	}
}
