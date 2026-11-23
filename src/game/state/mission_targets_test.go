package state

import (
	"encoding/binary"
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestSourceObjectiveCountdownHandBoundaries(t *testing.T) {
	for _, c := range []struct {
		d    int32
		want uint16
	}{{0, 0}, {75, 0}, {76, 1}, {2048, 27}, {786432, 10368}} {
		got, err := SourceObjectiveCountdown(MissionPoint{c.d, 0}, MissionPoint{})
		if err != nil || got != c.want {
			t.Fatal(c, got, err)
		}
	}
	if _, err := SourceObjectiveCountdown(MissionPoint{786433, 0}, MissionPoint{}); err == nil {
		t.Fatal("exceptional FP domain accepted")
	}
}

func TestFullTargetRecordFieldsAndOpaqueRetention(t *testing.T) {
	labels := &assetdata.OutfitObjectiveText{RecordFormat: "%02d/%02d %s %Fs", Verbs: []string{"Training", "S&D", "Destroy", "Destroy", "Pick up", "Drop off", "S&R", "Recon"}}
	fmhd := make([]byte, 39)
	copy(fmhd, "Tank Group")
	a := &assetdata.MissionAssets{Preflight: &assetdata.PreflightAssets{ObjectiveSummary: labels}}
	o := MissionObjective{Role: "primary", Formation: 0, RawKind: 3, DisplayKind: 3, Count: 7, Name: "presentation-only override", Point: MissionPoint{0x1c000, 0x2c000}, OriginalPoint: MissionPoint{0x1234, 0x5678}, Endpoint: &MissionPoint{0x334455, 0x667788}}
	m := &MissionData{SourceWorld: &assetdata.MissionWorld{FMHD: fmhd}, Objectives: map[string]MissionObjective{"primary": o}}
	if err := PrepareMissionTargets(a, m); err != nil {
		t.Fatal(err)
	}
	r := m.Targets.Records[2]
	wantName := "04/06 Destroy Tank Group"
	if string(r[:len(wantName)]) != wantName || r[len(wantName)] != 0 {
		t.Fatalf("source full name %q", r[:32])
	}
	if binary.LittleEndian.Uint16(r[32:]) != 3 || binary.LittleEndian.Uint16(r[50:]) != 7 {
		t.Fatal("raw/mapped kind or transformed count changed")
	}
	if binary.LittleEndian.Uint32(r[34:]) != 0x1234 || binary.LittleEndian.Uint32(r[42:]) != 0x1c000 || binary.LittleEndian.Uint32(r[54:]) != 0x334455 || binary.LittleEndian.Uint32(r[58:]) != 0x667788 {
		t.Fatal("coordinate domains/tails conflated")
	}
	raw := make([]byte, 310)
	for i := range raw {
		raw[i] = byte(i*17 + 9)
	}
	retained := &MissionData{SourcePreparation: &CampaignSourceRaw{TargetBlock: raw}}
	if err := PrepareMissionTargets(nil, retained); err != nil {
		t.Fatal(err)
	}
	for i, v := range raw {
		if retained.Targets.Records[i/62][i%62] != v {
			t.Fatalf("opaque byte%d lost", i)
		}
	}
	raw[61] ^= 255
	if retained.Targets.Records[0][61] == raw[61] {
		t.Fatal("retained TARGET aliases imported input")
	}
}
