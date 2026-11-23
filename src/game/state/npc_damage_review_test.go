package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestNPCIndependentInstructionOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("supported original source operand fixture unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if h := sha256.Sum256(raw); hex.EncodeToString(h[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported executable")
	}
	for ip, want := range map[int]string{
		0x7655: "a03c592ae43b46067403e9da03", // NPC skips player-only high masks.
		0x7af8: "2680bf0206027410",           // whole602 ==2 enters NPC failure.
		0x7b00: "2680bf0706027408",           // whole607 ==2, even partner healthy.
		0x7b08: "2680bf0006027534",           // main600 ==2.
		0x7b1b: "6b5efa1e2638871d007204",     // bucket-indexed alias, strict roll>alias.
		0x7b4d: "2680bf0506027506",           // whole tail605 ==2 ->6092.
		0x7bce: "26d1bf0500",                 // SAR signed16 ammo in literal SHIP3.
		0x7d68: "26d1bf0500",
		0xa845: "2680bf090601751d", //609==1 falling; any other nonzero descent.
		0x5c26: "26c6470800",       // shared service clears609 only.
	} {
		b, e := hex.DecodeString(want)
		if e != nil {
			t.Fatal(e)
		}
		got := raw[0xd8b0+ip : 0xd8b0+ip+len(b)]
		if !bytes.Equal(got, b) {
			t.Fatalf("0975:%04X: %x != %s", ip, got, want)
		}
	}
	writer, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if h := sha256.Sum256(writer); hex.EncodeToString(h[:]) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("unsupported writer executable")
	}
	for ip, want := range map[int]string{
		0x886: "6a186a008d86e4fe509a3a2cbf00",         // memset(local,0,24).
		0x89d: "837efc147d17",                         // pad through20 records.
		0x8a3: "ff36f6986a016a188d86e4fe509a2808bf00", // write24.
	} {
		b, _ := hex.DecodeString(want)
		if !bytes.Equal(writer[0x32e70+ip:][:len(b)], b) {
			t.Fatalf("GS1C89:%04X padding operand changed", ip)
		}
	}
}

func TestNPCCompactPaddingKnownZeroAndHistoricalUnknown(t *testing.T) {
	// Source constructor fixture, deliberately entirely unarmed. Ordinary
	// accepted port launches require an armed primary, which already provides
	// compact2; this exercises the source writer's otherwise absent record.
	a := initializationAssets(t)
	m, err := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionTraining, Theatre: TheatreCentralEurope, Rank: RankBrigadierGeneral, PilotName: "Padding fixture", Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	m.Support = nil
	var unarmed [36]byte
	found := false
	for id := 0; id < 8; id++ {
		r, e := InitializeReadinessRow(a.Missions, m, id)
		if e != nil {
			t.Fatal(e)
		}
		selector := a.Missions.Generation.Tables.Initialization.AircraftSelectors[id] & 7
		if int8(r[2]) < 0 && selector != 3 && selector != 5 {
			unarmed, found = r, true
			break
		}
	}
	if !found {
		t.Fatal("source catalogue lacks the known cannonless constructor")
	}
	for j := 0; j < 3; j++ {
		RowPut(unarmed[:], 12+2*j, 0)
	}
	rows := [][36]byte{unarmed, unarmed, unarmed, unarmed, unarmed}
	stores, err := BuildRuntimeStores(a.Missions, m, rows)
	if err != nil || len(stores.Definitions) != 0 {
		t.Fatal("unarmed real constructor must omit compact2", err, stores)
	}
	for i := range stores.Ships {
		p := byte(127)
		stores.Ships[i].Proficiency = &p
	}
	if err = stores.EnableNPCDamage(make([]byte, 80)); err != nil {
		t.Fatal(err)
	}
	if stores.NPCDamage.Known[9] {
		t.Fatal("old interpretation changed its unknown carrier")
	}
	if err = stores.EnableNPCZeroPaddedDefinitions(); err != nil || !stores.NPCDamage.Known[9] || stores.NPCDamage.BucketBytes[9] != 0 {
		t.Fatal("fresh source padding must establish literal0", err, stores.NPCDamage)
	}
	for _, roll := range []uint16{0, 1, 255} {
		mission, c := nPCFixture(t, 2, 172, roll)
		mission.Stores = stores
		mission.Entities[1].SerializedAircraftSelector = int(stores.Ships[1].Flags & 7)
		stores.Damage[1] = [10]byte{7: 1}
		out, e := mission.ApplyAcceptedNPCHit(c)
		want := byte(1)
		if roll > 0 {
			want = 2
		}
		if e != nil || out.Limit != "" || !out.Applied || out.RNGCalls != 2 || stores.Damage[1][9] != want {
			t.Fatal("known zero strict equality/greater branch", roll, out, e, stores.Damage[1])
		}
	}
	encoded, err := json.Marshal(stores)
	if err != nil || bytes.Contains(encoded, []byte("freshWeaponPadding")) {
		t.Fatal("transient constructor proof leaked into checkpoint", err)
	}
	var restored RuntimeStores
	if err = json.Unmarshal(encoded, &restored); err != nil || !restored.NPCDamage.Known[9] || restored.NPCDamage.BucketBytes[9] != 0 {
		t.Fatal("recorded known0 lost", err)
	}
	unknown, _ := nPCFixture(t, 2)
	unknown.Stores.Definitions = nil
	unknown.Stores.NPCDamage.Known[9] = false
	before := *unknown.Stores.NPCDamage
	if err = unknown.Stores.EnableNPCZeroPaddedDefinitions(); err == nil || !reflect.DeepEqual(before, *unknown.Stores.NPCDamage) {
		t.Fatal("unproven imported carrier must retain guard", err)
	}
}

func TestNPCAllComponentFirstAndCriticalHits(t *testing.T) {
	// Literal bucket ranges and explicit postconditions from7468..7EA3, not a
	// second copy of the production selector. Full indices are5FE..609.
	cases := []struct {
		name            string
		word            uint16
		index           int
		first, critical byte
		criticalDraw    []uint16
		fail            byte
		wreck           bool
	}{
		{"avionics", 0, 0, 1, 2, nil, 0, false},
		{"optics", 26, 1, 1, 2, nil, 0, false},
		{"main-rotor", 44, 2, 1, 2, []uint16{12}, 1, false},
		{"port-wing", 51, 3, 2, 2, nil, 0, false},
		{"port-engine", 77, 4, 1, 2, []uint16{14}, 1, false},
		{"fuel", 95, 5, 1, 2, nil, 0, true},
		{"cannon", 102, 6, 1, 2, nil, 0, false},
		{"tail-rotor", 128, 7, 1, 2, nil, 2, false},
		{"starboard-wing", 146, 8, 2, 2, nil, 0, false},
		{"starboard-engine", 172, 9, 1, 2, []uint16{99}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, critical := range []bool{false, true} {
				words := []uint16{tc.word}
				if critical {
					words = append(words, tc.criticalDraw...)
				}
				m, c := nPCFixture(t, 2, words...)
				if critical {
					if tc.index < 2 {
						m.Stores.NPCDamage.Masks[1][tc.index] = 1
					} else {
						m.Stores.Damage[1][tc.index-2] = 1
					}
				}
				beforePose := m.Entities[1].Pose
				out, err := m.ApplyAcceptedNPCHit(c)
				if err != nil || out.Limit != "" || !out.Applied || out.ReturnCode != uint16(tc.index+1) || !out.NotifyDamage || out.RNGCalls != uint32(len(words)) {
					t.Fatal(critical, out, err)
				}
				want := tc.first
				if critical {
					want = tc.critical
				}
				d := m.Stores.Damage12(1)
				if d[tc.index] != want || d[11] != map[bool]byte{true: tc.fail, false: 0}[critical] || out.DirectWreck != (critical && tc.wreck) {
					t.Fatal(critical, out, d)
				}
				if d[0]&0xf0 != 0 || d[1]&0xc0 != 0 || d[10] != 0 || m.Entities[1].Pose != beforePose || m.Stores.Damage12(0) != [12]byte{} {
					t.Fatal("NPC borrowed player effects", d)
				}
				if tc.index == 5 && m.Stores.Ships[1].Fuel != 98 {
					t.Fatal("source AND lowfuel bit", m.Stores.Ships[1].Fuel)
				}
			}
		})
	}
	// A previous full severity2 must reroll; repeated wing hits cannot increment3.
	for _, tc := range []struct {
		word  uint16
		index int
	}{{51, 3}, {146, 8}} {
		m, c := nPCFixture(t, 2, tc.word, 0)
		m.Stores.Damage[1][tc.index-2] = 2
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || out.Bucket != 0 || out.RNGCalls != 2 || m.Stores.Damage12(1)[tc.index] != 2 {
			t.Fatal(out, err)
		}
	}
}

func TestNPCEverySelectorRemapAndWholeByteBoundary(t *testing.T) {
	// Serialized selectors0/1 Apache extra draw;3/5 lack607;5/6 lack604.
	for selector := byte(0); selector < 8; selector++ {
		for _, choice := range []struct {
			word   uint16
			bucket int
		}{{0, 0}, {102, 6}, {172, 9}, {190, 10}} {
			words := []uint16{choice.word}
			if selector < 2 {
				words = append(words, choice.word)
			}
			want := choice.bucket
			if want == 10 || want == 6 && (selector == 5 || selector == 6) || want == 9 && (selector == 3 || selector == 5) {
				want = 0
				words = append(words, 0)
				if selector < 2 {
					words = append(words, 0)
				}
			}
			m, c := nPCFixture(t, selector, words...)
			out, err := m.ApplyAcceptedNPCHit(c)
			if err != nil || out.Bucket != int16(want) || out.RNGCalls != uint32(len(words)) {
				t.Fatal(selector, choice, out, err)
			}
		}
	}
	for _, tc := range []struct {
		before, after byte
		words         []uint16
		bucket        int16
	}{{0x80, 0x81, []uint16{0}, 0}, {0xff, 0, []uint16{0}, 0}, {0xfe, 0xfe, []uint16{0, 26}, 1}} {
		m, c := nPCFixture(t, 2, tc.words...)
		m.Stores.NPCDamage.Masks[1][0] = tc.before
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || out.Bucket != tc.bucket || m.Stores.Damage12(1)[0] != tc.after {
			t.Fatal(tc, out, err, m.Stores.Damage12(1))
		}
	}
	// Partial then critical engine becomes609; highflag82 is NOT fullbyte2.
	m, c := nPCFixture(t, 2, 77)
	m.Stores.Damage[1][2] = 0x81
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || out.RNGCalls != 1 || m.Stores.Damage[1][2] != 0x82 || m.Stores.Damage[1][9] != 0 {
		t.Fatal(out, err, m.Stores.Damage12(1))
	}
	// General failure counts firstTEN whole nonzero bytes; equality4 is healthy.
	for _, count := range []int{4, 5} {
		m, c := nPCFixture(t, 2, 0)
		m.Stores.NPCDamage.Masks[1] = [2]byte{1, 1}
		m.Stores.Damage[1][1] = 2
		m.Stores.Damage[1][4] = 1
		if count == 5 {
			m.Stores.Damage[1][6] = 2
		}
		out, err := m.ApplyAcceptedNPCHit(c)
		want := byte(0)
		if count == 5 {
			want = 1
		}
		if err != nil || !out.Applied || m.Stores.Damage[1][9] != want {
			t.Fatal(count, out, err, m.Stores.Damage12(1))
		}
	}
}

func TestNPCSignedWingAmmoServiceAndSnapshot(t *testing.T) {
	m, c := nPCFixture(t, 2, 51)
	m.Stores.Ships[1].Stations[1] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 101}
	m.Stores.Ships[3].Stations[1] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: -3}
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || !out.Applied || m.Stores.Ships[3].Stations[1].Ammunition != -2 || m.Stores.Ships[1].Stations[1].Ammunition != 101 {
		t.Fatal(out, err, m.Stores.Ships)
	}
	// Service is rearm/refuel, not repairing the physical record or two masks.
	d := [10]byte{2, 2, 2, 1, 2, 2, 0, 2, 0, 2}
	m.Stores.Damage[1] = d
	m.Stores.NPCDamage.Masks[1] = [2]byte{0x42, 0x82}
	m.Stores.Ships[1].Flags |= 0x18
	m.Stores.Blueprint = append([]RuntimeAircraft(nil), m.Stores.Ships...)
	m.Stores.Blueprint[1].Fuel = 100
	m.Stores.Blueprint[1].Stations[1].Ammunition = 101
	if err = m.Stores.Refill(1, &m.Stores.Damage[1]); err != nil {
		t.Fatal(err)
	}
	d[9] = 0
	if m.Stores.Damage[1] != d || m.Stores.NPCDamage.Masks[1] != [2]byte{0x42, 0x82} || m.Stores.Ships[1].Stations[1].Ammunition != 50 || m.Stores.Ships[1].Fuel != 100 || m.Stores.Ships[1].Flags&0x18 != 0x18 {
		t.Fatal(m.Stores.Damage12(1), m.Stores.Ships[1])
	}
	raw, err := json.Marshal(m.Stores)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeStores
	if err = json.Unmarshal(raw, &restored); err != nil || !reflect.DeepEqual(m.Stores.Damage12(1), restored.Damage12(1)) || !reflect.DeepEqual(m.Stores.Ships, restored.Ships) {
		t.Fatal("complete damaged state not preserved", err)
	}
}
