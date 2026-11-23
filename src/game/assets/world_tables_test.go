package assets

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestInitializedWorldPreparedTablesAgainstOriginal(t *testing.T) {
	path := os.Getenv("GS2000_TEST_PREPARED")
	if path == "" {
		t.Skip("set separate StepI prepared tree")
	}
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original source missing")
	}
	if e != nil {
		t.Fatal(e)
	}
	if Hash(raw) != IslandIce469085.Source(RoleSimulationExecutable).SHA256 {
		t.Fatal("unsupported original identity")
	}
	b, e := LoadWorldAssets(path)
	if e != nil {
		t.Fatal(e)
	}
	// These independently fixed raw coordinates come from source2814 operands,
	// not the production profile offsets or an inverse of the asset exporter.
	for i, v := range b.AngleBases {
		if v != binary.LittleEndian.Uint16(raw[0x4c452+2*i:]) {
			t.Fatal("angle base", i)
		}
	}
	for i, v := range b.Atan {
		if v != binary.LittleEndian.Uint16(raw[0x4c462+2*i:]) {
			t.Fatal("angle ratio", i)
		}
	}
	front, e := LoadFrontEndAssets(path)
	if e != nil {
		t.Fatal(e)
	}
	defaults := front.Missions.Generation.Tables.Initialization.InitialViewWords
	want := [6]int16{0, 0, 0, 5, 128, 0}
	if defaults != want {
		t.Fatal("source cold view/HUD words", defaults)
	}
	for i, offset := range [6]int{0xacf, 0xad1, 0xad7, 0xad9, 0xadf, 0xae7} {
		if defaults[i] != int16(binary.LittleEndian.Uint16(raw[0x487f0+offset:])) {
			t.Fatal("source view word", i)
		}
	}
	if !bytes.Equal(b.WeatherRemap, raw[0x4b552:0x4b652]) {
		t.Fatal("weather destination lookup")
	}
	if Hash(raw[0x4c452:0x4c462]) != "d57d75d4bd8f68eb1f48d2cfa9a467f5b363573077a2733cc47dd1dafb85bb78" || Hash(raw[0x4c462:0x4e462]) != "d0db330f9d3ad1f470017c285111b4dcc1529c38e3b8c041ea4284f9ad58dafd" {
		t.Fatal("source table identity")
	}
}
