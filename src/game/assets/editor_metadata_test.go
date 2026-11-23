package assets

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"

	"testing"
)

func TestEditorSourceTablesAndCurrentProfile(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_EDITOR")
	if dir == "" {
		t.Skip("requires declared current Preparation27 fixture")
	}
	a, e := LoadFrontEndAssets(dir)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	capabilityErr := ValidateCurrentFrontEndCapabilities(a)
	if m.PreparationVersion >= 34 && capabilityErr != nil {
		t.Fatal(capabilityErr)
	}
	if m.PreparationVersion >= 27 && m.PreparationVersion <= 33 && capabilityErr == nil {
		t.Fatal("historical27..33 must load but require current capabilities")
	}

	x := a.Editor
	if len(x.FillLabels) != 6 || len(x.DeleteLabels) != 19 || x.FillLabels[0] != "Random Neutral" || x.FillLabels[5] != "Rear Echelon" || x.DeleteLabels[18] != "All defenders" {
		t.Fatal("source menus", x.FillLabels, x.DeleteLabels)
	}
	if len(x.ModelNames) != 230 || len(x.ModelInformation) != 229 || len(x.ModelInformationRaw) != 230 || x.ModelInformation[124] != "" || len(x.ModelInformationRaw[124]) == 0 {
		t.Fatal("source help bounds/anomalous original index")
	}
	for _, p := range []string{"GULF", "EURO", "PHIL", "ANTC"} {
		if len(x.WeaponTypes[p]) != 16*27 {
			t.Fatal(p)
		}
	}
	h, e := LogicalResourceProfile(dir)
	want := map[int]string{27: LogicalPreparation27, 28: LogicalPreparation28, 29: LogicalPreparation29, 30: LogicalPreparation30, 31: LogicalPreparation31, 32: LogicalPreparation32, 33: LogicalPreparation33, 34: LogicalPreparation34}[m.PreparationVersion]
	if e != nil || want == "" || h != want {
		t.Fatal(h, e, m.PreparationVersion)
	}
}
func TestEditorIndependentObjectInformationOperands(t *testing.T) {
	p := testfixture.Path("installation/OBJDBASE.GS2")
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		t.Skip("original bytes external prerequisite")
	}
	if e != nil {
		t.Fatal(e)
	}
	if len(b) != 19658 || binary.LittleEndian.Uint16(b[:]) != 1840 || binary.LittleEndian.Uint16(b[2:]) != 19354 || binary.LittleEndian.Uint16(b[124*8+2:]) != 1774 {
		t.Fatal("original index operands")
	}
	if string(b[19354:19354+36]) != "Friendly attack aircraft. No threat." {
		t.Fatal("original description")
	}

}
