package state

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestLaunchSwitchesPreserveConditionsAndSourceDomains(t *testing.T) {
	mapping := []byte{3, 1, 0}
	m := &MissionData{Inputs: MissionInputs{MissionType: MissionTraining}, Conditions: MissionConditions{Flags: 0x300}}
	got, e := LaunchSwitches(m, PreflightSettings{}, mapping)
	// GS8ca: CMP1/SBB/NEG sets20,40,0400 for the zero ordinals.
	// CP/G0 maps3→18; CLEAR removes100; DARK200 survives:0678.
	if e != nil || got != 0x0678 {
		t.Fatalf("default launch%04x/%v", got, e)
	}
	m.Inputs.MissionType = MissionCampaign
	m.Scene = 2
	s := PreflightSettings{CPGControls: 2, EnemyQuality: 3, FlightModel: 1, Landings: 1, Avoidance: 1, Wind: 1, Visibility: 1}
	got, e = LaunchSwitches(m, s, mapping)
	if e != nil || got != 0x9387 {
		t.Fatalf("carrier launch%04x/%v", got, e)
	}
	m.Conditions.Flags |= 0x800
	got, e = LaunchSwitches(m, s, mapping)
	if e != nil || got != 0x8b87 {
		t.Fatalf("pass-through overrides carrier%04x/%v", got, e)
	}
	if m.Conditions.Flags != 0xb00 {
		t.Fatal("launch modified mission conditions")
	}
	m.Inputs.MissionType = MissionSimulated
	// Step P deliberately admits raw2. Source mission bits4000 replace8000;
	// pass-through0800 remains and Training protection0400 is clear.
	got, e = LaunchSwitches(m, s, mapping)
	if e != nil || got != 0x4b87 {
		t.Fatalf("source Simulation launch%04x/%v", got, e)
	}
}

func TestLaunchCPGMappingFromOriginalSource(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional source missing")
	}
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(raw[0x41034:0x41037], []byte{3, 1, 0}) {
		t.Fatal("source CPG selector table")
	}
	for key, want := range []uint16{0x18, 0x08, 0} {
		m := &MissionData{Inputs: MissionInputs{MissionType: MissionSingle}}
		got, e := LaunchSwitches(m, PreflightSettings{CPGControls: key}, raw[0x41034:0x41037])
		if e != nil || got != want|0x2060 {
			t.Fatal(key, got, e)
		}
	}
}
