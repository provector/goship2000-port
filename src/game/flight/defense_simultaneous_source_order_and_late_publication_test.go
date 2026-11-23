package flight

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"

	"gunship2000-assets/game/world"
)

func defenseFixture() (Defenses, R3DefenseRequest) {
	d := NewDefenses(30, 30)
	d.EnableR3()
	a := &world.Bundle{Sine: make([]int16, 1025)}
	a.Sine[1024] = 16384
	return d, R3DefenseRequest{Switches: 8, E0: true, Phase: 2, Threats: 0xc0,
		Pose: Pose{H0: 1000, H1: 2000, Height: 100}, DF: 6, Angles: a, SoundEnabled: true}
}

func TestDefenseSimultaneousSourceOrderAndLatePublication(t *testing.T) {
	d, in := defenseFixture()
	before := d
	if _, disabled := d.GuidanceResponse(0xc3, 0); disabled {
		t.Fatal("late defense influenced an earlier seeker visit")
	}
	out, err := d.ApplyR3Automatic(in, func() uint16 { t.Fatal("decoys consume no RNG"); return 0 })
	if err != nil || !out.ConsumedAEB || !reflect.DeepEqual(out.Deployments, []DefenseAction{LaunchFlare, LaunchChaff}) || !bytes.Equal(out.SoundRequests, []byte{0x2a, 0x0a}) || out.StatusPointer != 0x30ce {
		t.Fatalf("source order: %+v %v", out, err)
	}
	if d.Flares != 27 || d.Chaff != 27 || d.FlareTimer != 10 || d.ChaffTimer != 10 || d.Decoy != in.Pose || d.DriftH0 != 0 || d.DriftH1 != -72 || d.DriftHeight != -6 {
		t.Fatalf("shared final chaff state: %+v", d)
	}
	if before.Flares != 30 || before.R3 == d.R3 {
		t.Fatal("value checkpoint changed through owner alias")
	}
	d.AdvanceDecoy()
	if d.Decoy.H0 != 1000 || d.Decoy.H1 != 1928 || d.Decoy.Height != 94 {
		t.Fatal("shared decoy must drift once after both arms", d)
	}
	// No early counters or drift were secretly run inside 58F2.
	d.AdvanceCounters(true)
	if d.FlareTimer != 9 || d.ChaffTimer != 9 {
		t.Fatal("early counters must remain a separate once-per-visit phase")
	}
}

func TestDefenseSignedStoresTimerAndFallbackDistinction(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		stock, timer, after, nextTimer int16
		active                         uint16
		cue                            bool
	}{
		{"last cartridge", 1, 0, -2, 10, 0, true},
		{"two cartridges", 2, 0, -1, 10, 0, true},
		{"exact group", 3, 0, 0, 10, 0, true},
		{"positive active effect", 3, 4, 3, 4, 0, false},
		{"positive negative timer still active", 3, -1, 3, -1, 0, false},
		{"empty active effect", 0, 4, 0, 4, 64, false},
		{"negative empty active effect", -2, 4, -2, 4, 64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, in := defenseFixture()
			d.Flares, d.FlareTimer, in.Threats = tc.stock, tc.timer, 0x80
			out, err := d.ApplyR3Automatic(in, nil)
			if err != nil || !out.ConsumedAEB || d.Flares != tc.after || d.FlareTimer != tc.nextTimer || d.IRActive != tc.active || (len(out.SoundRequests) == 1) != tc.cue {
				t.Fatalf("independent branch expectation %+v result%+v err%v", d, out, err)
			}
			if tc.cue && (out.StatusPointer != 0x3078 || out.SoundRequests[0] != 0x2a) {
				t.Fatal("flare source status/cue", out)
			}
		})
	}
}

func TestDefenseRadarSignedTimerAndBothEnabledMasks(t *testing.T) {
	for _, mask := range []uint16{8, 0x18} {
		for _, tc := range []struct {
			stock, timer, after, nextTimer int16
			token                          uint16
			draws                          int
		}{
			{1, 0, -2, 10, 0, 0},
			{3, 4, 3, 4, 0, 0},
			{3, -1, 3, -1, 0, 0},
			{0, 4, 0, 4, 6, 1},
			{-2, 4, -2, 4, 6, 1},
		} {
			d, in := defenseFixture()
			d.Chaff, d.ChaffTimer = tc.stock, tc.timer
			in.Switches, in.Threats = mask, 0x40
			draws := 0
			out, err := d.ApplyR3Automatic(in, func() uint16 { draws++; return 5 })
			if err != nil || !out.ConsumedAEB || d.Chaff != tc.after || d.ChaffTimer != tc.nextTimer || d.RadarSignature != tc.token || draws != tc.draws || d.Flares != 30 {
				t.Fatalf("mask%02X independent radar expectation%+v result%+v draws%d err%v", mask, tc, d, draws, err)
			}
		}
	}
}

func TestDefenseHeldManualBitsCannotDeployAutomaticDecoys(t *testing.T) {
	d, in := defenseFixture()
	if err := d.SetR3ManualHeld([2]bool{true, true}, 0, func() uint16 { return 5 }); err != nil {
		t.Fatal(err)
	}
	in.Threats = 0 // no physical AEB even though both accepted inputs are held
	out, err := d.ApplyR3Automatic(in, nil)
	if err != nil || !out.ConsumedAEB || len(out.Deployments) != 0 || len(out.SoundRequests) != 0 || d.Chaff != 30 || d.Flares != 30 || d.ChaffTimer != 0 || d.FlareTimer != 0 || d.IRActive != 64 || d.RadarSignature != 6 || d.R3.Automatic != [2]bool{} {
		t.Fatal("physical input was converted into incoming threat", d, out, err)
	}
}

func TestDefenseManualRepeatCompetesOnlyAtLateArm(t *testing.T) {
	for _, stock := range []int16{30, 1, 0} {
		d, in := defenseFixture()
		d.Flares, d.FlareTimer, in.Threats = stock, 7, 0x80
		accepted := d.Activate(LaunchFlare, 0, in.Pose, in.DF, in.Angles, nil)
		out, err := d.ApplyR3Automatic(in, nil)
		if err != nil || accepted != (stock > 0) || len(out.SoundRequests) != 0 || len(out.Deployments) != 0 {
			t.Fatal("automatic duplicated a manual release", stock, d, out, err)
		}
		switch stock {
		case 30:
			if d.Flares != 27 || d.FlareTimer != 10 || d.IRActive != 0 {
				t.Fatal(d)
			}
		case 1:
			if d.Flares != -2 || d.FlareTimer != 10 || d.IRActive != 64 {
				t.Fatal(d)
			}
		case 0:
			if d.Flares != 0 || d.FlareTimer != 7 || d.IRActive != 64 {
				t.Fatal(d)
			}
		}
	}
	d, in := defenseFixture()
	in.Threats = 0x40
	if !d.Activate(LaunchFlare, 0, Pose{Height: 500}, in.DF, in.Angles, nil) {
		t.Fatal("manual flare")
	}
	out, err := d.ApplyR3Automatic(in, nil)
	if err != nil || d.Flares != 27 || d.Chaff != 27 || d.Decoy != in.Pose || d.DriftHeight != -6 || out.StatusPointer != 0x30ce {
		t.Fatal("manual flare/automatic chaff did not share last-write history", d, out, err)
	}
	// Repeated manual deployment remains accepted while the timer is active.
	if !d.Activate(LaunchChaff, 0, in.Pose, in.DF, in.Angles, nil) || d.Chaff != 24 {
		t.Fatal("new mode changed manual repeat")
	}
}

func TestDefenseSkippedVisitsRetainOwnersAndPhysicalLatch(t *testing.T) {
	for _, gate := range []string{"none", "E0", "odd", "damage"} {
		d, in := defenseFixture()
		d.Chaff, d.Flares = 0, 0
		if _, err := d.ApplyR3Automatic(in, func() uint16 { return 5 }); err != nil {
			t.Fatal(err)
		}
		before := d
		in.Threats = 0 // no new live slots; the caller's physical latch can persist
		switch gate {
		case "none":
			in.Switches = 0
		case "E0":
			in.E0 = false
		case "odd":
			in.Phase = 3
		case "damage":
			in.Damage = 0x40
		}
		out, err := d.ApplyR3Automatic(in, func() uint16 { t.Fatal("skipped RNG"); return 0 })
		if err != nil || out.ConsumedAEB || !reflect.DeepEqual(before, d) {
			t.Fatal(gate, d, out, err)
		}
		if err := d.SetR3ManualHeld([2]bool{}, 0, nil); err != nil || d.IRActive != 64 || d.RadarSignature != 6 {
			t.Fatal("release erased automatic continuation", gate, d, err)
		}
		in.Switches, in.E0, in.Phase, in.Damage = 8, true, 4, 0
		out, err = d.ApplyR3Automatic(in, nil)
		if err != nil || !out.ConsumedAEB || d.IRActive != 0 || d.RadarSignature != 0 {
			t.Fatal("eligible absent bits failed clear", gate, d, err)
		}
	}
	// An expired threat remains actionable when the caller supplies retained AEB.
	d, in := defenseFixture()
	in.Threats, in.Phase = 0x80, 3
	if out, _ := d.ApplyR3Automatic(in, nil); out.ConsumedAEB {
		t.Fatal("odd consumed latch")
	}
	in.Phase = 4
	if out, err := d.ApplyR3Automatic(in, nil); err != nil || !out.ConsumedAEB || d.Flares != 27 {
		t.Fatal("retained expired threat", d, out, err)
	}
}

func TestDefenseSingleTokenChargeAndOwnerTransfers(t *testing.T) {
	d, in := defenseFixture()
	d.Chaff, d.Flares, d.IRCharge = 0, 0, 2
	draws := 0
	rng := func() uint16 { draws++; return 13 }
	if err := d.SetR3ManualHeld([2]bool{true, true}, 0, rng); err != nil {
		t.Fatal(err)
	}
	checkpoint := d
	d.AdvanceCounters(true)
	if _, err := d.ApplyR3Automatic(in, rng); err != nil {
		t.Fatal(err)
	}
	if draws != 1 || d.RadarSignature != 6 || d.IRActive != 1 || d.IRCharge != 1 {
		t.Fatal("auto cloned manual charge/token", d, draws)
	}
	if err := d.SetR3ManualHeld([2]bool{}, 0, rng); err != nil || d.IRActive != 1 || d.RadarSignature != 6 {
		t.Fatal("manual release killed auto", d, err)
	}
	d.AdvanceCounters(true) // shared charge exhausted; ownership may not resurrect2
	if d.IRActive != 0 || d.IRCharge != 0 {
		t.Fatal(d)
	}
	if err := d.SetR3ManualHeld([2]bool{true, true}, 0, rng); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ApplyR3Automatic(in, rng); err != nil {
		t.Fatal(err)
	}
	if d.IRActive != 0 || d.IRCharge != 0 || draws != 1 {
		t.Fatal("exhausted fallback replenished on owner change", d, draws)
	}
	// Absent physical AEB clears automatic, but held manual input still sustains
	// the same token. Held bits never manufacture a decoy deployment.
	in.Threats = 0
	if out, err := d.ApplyR3Automatic(in, rng); err != nil || len(out.Deployments) != 0 || d.RadarSignature != 6 || d.R3.Automatic != [2]bool{} {
		t.Fatal(d, out, err)
	}
	if err := d.SetR3ManualHeld([2]bool{}, 0, rng); err != nil || d.RadarSignature != 0 {
		t.Fatal(d, err)
	}
	if err := d.SetR3ManualHeld([2]bool{true, false}, 0, rng); err != nil || draws != 2 {
		t.Fatal("real off/on must draw", d, draws, err)
	}
	if checkpoint.R3.Automatic != [2]bool{} || checkpoint.R3.ManualHeld != [2]bool{true, true} || checkpoint.IRCharge != 2 {
		t.Fatal("later ownership mutated copied checkpoint")
	}
	// Genuine early inactive recharge remains, and occurs exactly once here.
	d.AdvanceCounters(true)
	if d.IRCharge != 1 {
		t.Fatal("owner change took over natural counter recharge", d)
	}
}

func TestDefenseDamagedSuppressionAndInactiveOnlyDependencies(t *testing.T) {
	d, in := defenseFixture()
	d.Chaff, d.Flares = 0, 0
	draws := 0
	rng := func() uint16 { draws++; return 3 }
	if _, err := d.ApplyR3Automatic(in, rng); err != nil {
		t.Fatal(err)
	}
	if err := d.SetR3ManualHeld([2]bool{}, 0x40, nil); err != nil || d.IRActive != 0 || d.RadarSignature != 0 || d.R3.Automatic != [2]bool{true, true} {
		t.Fatal(d, err)
	}
	if err := d.SetR3ManualHeld([2]bool{}, 0, nil); err != nil || d.IRActive != 64 || d.RadarSignature != 4 || draws != 1 {
		t.Fatal("damage recovery duplicated a token", d, draws, err)
	}
	if effective := d.EffectiveR3(0x40); effective.IRActive != 0 || effective.RadarSignature != 0 || d.IRActive != 64 {
		t.Fatal("effective suppression mutated canonical decision")
	}
	// Existing token needs no RNG. Inactive fallback needs a source draw and
	// must leave both branches untouched if the dependency is missing.
	if _, err := d.ApplyR3Automatic(in, nil); err != nil {
		t.Fatal(err)
	}
	d, in = defenseFixture()
	d.Chaff, d.Flares = 0, 0
	before := d
	if out, err := d.ApplyR3Automatic(in, nil); err == nil || out.ConsumedAEB || !reflect.DeepEqual(d, before) {
		t.Fatal("missing RNG accepted/partially mutated", d, out, err)
	}
	d, in = defenseFixture()
	in.Angles = nil
	before = d
	if out, err := d.ApplyR3Automatic(in, rng); err == nil || out.ConsumedAEB || !reflect.DeepEqual(d, before) || draws != 1 {
		t.Fatal("missing angles accepted/partially mutated", d, out, err)
	}
	d, in = defenseFixture()
	in.SoundEnabled = false
	if out, err := d.ApplyR3Automatic(in, nil); err != nil || len(out.SoundRequests) != 0 || len(out.Deployments) != 2 {
		t.Fatal("DE gates requests, not mechanics", out, err)
	}
}

func TestDefenseHistoricalJSONAndOwnershipRestore(t *testing.T) {
	legacy := NewDefenses(30, 30)
	raw, err := json.Marshal(legacy)
	if err != nil || bytes.Contains(raw, []byte(`"R3"`)) {
		t.Fatal("historical JSON acquired new zero state", string(raw), err)
	}
	d, in := defenseFixture()
	if d.R3.StatusPointer != 0x3078 {
		t.Fatal("initialized mainDS406A decoy descriptor pointer differs")
	}
	d.Chaff, d.Flares = 0, 0
	if _, err := d.ApplyR3Automatic(in, func() uint16 { return 7 }); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored Defenses
	if err := json.Unmarshal(raw, &restored); err != nil || !reflect.DeepEqual(restored, d) {
		t.Fatal("owner checkpoint round trip", err)
	}
	if err := restored.SetR3ManualHeld([2]bool{}, 0, nil); err != nil || restored.RadarSignature != 8 || restored.IRActive != 64 {
		t.Fatal("resume cleanup erased auto", restored, err)
	}
	if before := legacy; !reflect.DeepEqual(before.EffectiveR3(0x40), legacy) {
		t.Fatal("old profile publication reinterpreted")
	}
}

func TestDefenseSharedDecoySelectorCheckpointAndMixedArms(t *testing.T) {
	d, in := defenseFixture()
	before := d
	if !d.Activate(LaunchChaff, 0, in.Pose, in.DF, in.Angles, nil) || d.R3.StatusPointer != 0x30ce || before.R3.StatusPointer != 0x3078 {
		t.Fatal("manual R3 descriptor selection mutated copied checkpoint")
	}
	if !d.Activate(LaunchFlare, 0, in.Pose, in.DF, in.Angles, nil) || d.R3.StatusPointer != 0x3078 {
		t.Fatal("manual flare did not select the shared flare descriptor")
	}
	d, in = defenseFixture()
	d.Chaff = 0 // flare deploys/copies owner, then radar fallback sets its intent
	out, err := d.ApplyR3Automatic(in, func() uint16 { return 5 })
	if err != nil || d.R3.StatusPointer != 0x3078 || out.StatusPointer != 0x3078 || !d.R3.Automatic[0] || d.RadarSignature != 6 {
		t.Fatal("shared model write lost the later fallback owner", d, out, err)
	}
	d, in = defenseFixture()
	d.Flares = 0 // IR fallback survives the later chaff owner copy
	out, err = d.ApplyR3Automatic(in, nil)
	if err != nil || d.R3.StatusPointer != 0x30ce || out.StatusPointer != 0x30ce || !d.R3.Automatic[1] || d.IRActive != 64 {
		t.Fatal("later shared model write lost the earlier fallback owner", d, out, err)
	}
}

func TestDefenseOriginalDecisiveInstructions(t *testing.T) {
	// Reuse the established owned copy. These are static0975:IP coordinates,
	// fileD8B0+IP, not live DOS CS or unverified decompiler statements.
	raw, err := os.ReadFile(testfixture.Path("source-image.gs2"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unrecognized original owned-copy identity")
	}
	for _, tc := range []struct {
		ip    int
		bytes string
	}{
		{0x595c, "f606e43b08"}, {0x596a, "26f606fe0540"}, {0x5975, "803ee00000"}, {0x597f, "f606da0001"},
		{0x5996, "26f60780"}, {0x59a3, "26833e1b0000"}, {0x59a9, "7f03"}, {0x59b2, "26833ef50a00"},
		{0x59c1, "26832e1b0003"}, {0x59cb, "26c706f50a0a00"}, {0x5a37, "c70634317830"}, {0x5a44, "6a2a"},
		{0x5a54, "26833ef90a00"}, {0x5a60, "26a1142d"}, {0x5a7c, "26f60740"}, {0x5a89, "26833e190000"},
		{0x5a98, "26833ef70a00"}, {0x5aa7, "26832e190003"}, {0x5ab1, "26c706f70a0a00"},
		{0x5b1b, "c7063431ce30"}, {0x5b28, "6a0a"}, {0x5b38, "26833efb0a00"}, {0x5b40, "9a38207f03"},
		{0x5b45, "25070040"}, {0x5b62, "26c7070000"},
	} {
		want, _ := hex.DecodeString(tc.bytes)
		if !bytes.Equal(raw[0xd8b0+tc.ip:0xd8b0+tc.ip+len(want)], want) {
			t.Fatalf("original0975:%04X bytes differ", tc.ip)
		}
	}
	if !bytes.Equal(raw[0x61bc0+0x3134:0x61bc0+0x3136], []byte{0x78, 0x30}) {
		t.Fatal("mainDS406A:3134 initialized shared decoy pointer differs")
	}
}
