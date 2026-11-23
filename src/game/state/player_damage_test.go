package state

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"reflect"
	"strings"
	"testing"
)

func damageFixture(t *testing.T, selector byte, words ...uint16) (*InitializedMission, PlayerHitContext) {
	t.Helper()
	m := &InitializedMission{PlayerRow: 0, Switches: 2,
		PlayerFlight: &flight.State{RadarAltitude: 3},
		Entities:     []InitializedEntity{{RuntimeRow: 0, SerializedAircraftSelector: int(selector)}},
		Stores:       &RuntimeStores{Ships: []RuntimeAircraft{{Flags: selector, Fuel: 99}}, Damage: make([][10]byte, 1)},
	}
	systems := flight.NewSystems([]int16{99}, []bool{true})
	mass, speech, overlay := int16(8000), true, [2][2]int16{}
	n := 0
	c := PlayerHitContext{VictimRow: 0, Systems: &systems, MassTerm: &mass, SpeechEnabled: &speech, Overlay: &overlay, Random: func() uint16 {
		if n >= len(words) {
			t.Fatalf("unjustified extra RNG draw%d beyond original hand-case%v", n, words)
		}
		v := words[n]
		n++
		return v
	}}
	return m, c
}

func TestPlayerAcceptedHitProtectionQualityAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    uint16
		rng  []uint16
		want bool
	}{{"q0-75-suppressed", 0, []uint16{75}, false}, {"q0-76-accepted", 0, []uint16{76, 77}, true}, {"q1-37-suppressed", 1, []uint16{37}, false}, {"q1-38-accepted", 1, []uint16{38, 77}, true}, {"q2-no-quality-draw", 2, []uint16{77}, true}, {"q3-no-quality-draw", 3, []uint16{77}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			m, c := damageFixture(t, 2, tc.rng...)
			m.Switches = tc.q
			got, err := m.ApplyAcceptedPlayerHit(c)
			if err != nil || got.Applied != tc.want || int(got.RNGCalls) != len(tc.rng) {
				t.Fatal(got, err)
			}
			if tc.want && (got.Bucket != 4 || got.ReturnCode != 5 || c.Systems.EngineTargets[0] != 7) {
				t.Fatal("source branch/engine consumer", got, c.Systems.EngineTargets)
			}
		})
	}
	for _, mode := range []string{"protection", "terminal", "wrong-victim", "wrong-selector"} {
		m, c := damageFixture(t, 2)
		c.Random = func() uint16 { t.Fatal("early guard consumed gameplay RNG"); return 0 }
		switch mode {
		case "protection":
			m.Switches |= 0x400
		case "terminal":
			c.Systems.Damage[9] = 1
		case "wrong-victim":
			c.VictimRow = 1
		case "wrong-selector":
			m.Entities[0].SerializedAircraftSelector = 3
		}
		got, err := m.ApplyAcceptedPlayerHit(c)
		if got.Applied || got.RNGCalls != 0 || (err != nil) != strings.HasPrefix(mode, "wrong-") {
			t.Fatal(mode, got, err)
		}
	}
}

func TestPlayerDamageSourceSelectionRemapsAndRerolls(t *testing.T) {
	for _, selector := range []byte{0, 1} {
		m, c := damageFixture(t, selector, 10, 44)
		got, err := m.ApplyAcceptedPlayerHit(c)
		if err != nil || got.Bucket != 2 || got.RNGCalls != 2 || c.Systems.Damage[0] != 1 {
			t.Fatal("B21 adjacent9442 reroll", selector, got, err)
		}
	}
	for _, selector := range []byte{5, 6} {
		m, c := damageFixture(t, selector, 102, 0, 31)
		got, err := m.ApplyAcceptedPlayerHit(c)
		if err != nil || got.Bucket != 10 || *c.Overlay != [2][2]int16{{32, 71}, {}} || c.Systems.Damage[4] != 0 {
			t.Fatal("no cannon remap", selector, got, err)
		}
	}
	for _, selector := range []byte{3, 5} {
		m, c := damageFixture(t, selector, 172, 255, 0)
		got, err := m.ApplyAcceptedPlayerHit(c)
		if err != nil || got.Bucket != 10 || *c.Overlay != [2][2]int16{{287, 40}, {}} || c.Systems.Damage[7] != 0 {
			t.Fatal("no starboard engine remap", selector, got, err)
		}
	}
	m, c := damageFixture(t, 2, 77, 102)
	c.Systems.Damage[2] = 2
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.Bucket != 6 || got.RNGCalls != 2 || c.Systems.Damage[4] != 1 {
		t.Fatal("already-critical low2 reroll", got, err)
	}
}

func TestPlayerDamageMasksCriticalCountAndHealthDomains(t *testing.T) {
	m, c := damageFixture(t, 2, 0)
	c.Systems.DamageMasks[0] = 0x81
	c.Systems.DamageMasks[1] = 1
	c.Systems.Damage[0], c.Systems.Damage[1], c.Systems.Damage[2] = 1, 1, 1
	m.PlayerFlight.Hover, m.PlayerFlight.HeightControl = 1, 1
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || !got.Applied || c.Systems.DamageMasks[0] != 0xf2 || c.Systems.Damage[9] != 0 || m.PlayerFlight.Hover != 0 || m.PlayerFlight.HeightControl != 0 {
		t.Fatal("full byte82 is not full byte2; critical mask control consumers", got, err, c.Systems)
	}
	// A plain byte becoming2 DOES count nonzero full first-ten bytes, including
	// high-only masks. Five exceeds the non-Apache threshold4.
	m, c = damageFixture(t, 2, 77)
	c.Systems.DamageMasks = [2]byte{0x80, 0x40}
	c.Systems.Damage[0], c.Systems.Damage[1], c.Systems.Damage[2] = 1, 1, 1
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || c.Systems.Damage[9] != 1 || !reflect.DeepEqual(got.Audio, []uint16{0x3d}) || *c.SpeechEnabled || c.Systems.EngineTargets[0] != 0 {
		t.Fatal("full-byte count/warning source gate", got, err)
	}
	for _, tc := range []struct {
		d    [12]byte
		want uint16
	}{{[12]byte{}, 255}, {[12]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, 125}, {[12]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 2}, 255}, {[12]byte{2, 2, 2, 2}, 151}, {[12]byte{0xf2, 0xc2}, 203}} {
		if got := EncodedDamageHealth(tc.d); got != tc.want {
			t.Fatal("independent13*(20-sumlow2)-5 boundary", got, tc.want)
		}
	}
}

func TestPlayerDamageWarningBoundariesAndServiceDoesNotHeal(t *testing.T) {
	for _, tc := range []struct {
		altitude, countdown int16
		speech              bool
		want                bool
	}{{2, 0, true, false}, {3, 0, true, true}, {3, 1, true, false}, {3, 0, false, false}} {
		m, c := damageFixture(t, 3, 77)
		m.PlayerFlight.RadarAltitude = tc.altitude
		m.PlayerFlight.Terminal = &flight.Terminal{Countdown: tc.countdown}
		*c.SpeechEnabled = tc.speech
		c.Systems.Damage[2] = 1
		got, err := m.ApplyAcceptedPlayerHit(c)
		if err != nil || (len(got.Audio) == 1) != tc.want || *c.SpeechEnabled != (tc.speech && !tc.want) {
			t.Fatal("DE80/countdown/clearance warning boundary", tc, got, err)
		}
		c.Random = func() uint16 { t.Fatal("609 repeated hit consumed RNG"); return 0 }
		second, err := m.ApplyAcceptedPlayerHit(c)
		if err != nil || second.Applied || len(second.Audio) != 0 {
			t.Fatal("terminal repeated warning/hit", second, err)
		}
	}
	m, c := damageFixture(t, 2, 51)
	m.Stores.Ships[0].Flags |= 0x18
	m.Stores.Ships[0].Stations[1] = RuntimeStation{CompactID: 2, Ammunition: 11, Mode: 2}
	m.Stores.Definitions = []RuntimeWeapon{{CompactID: 2}}
	m.Stores.Blueprint = append([]RuntimeAircraft(nil), m.Stores.Ships...)
	c.Systems.DamageMasks[0] = 0x81
	c.Systems.Damage[2] = 1
	c.Systems.EngineTargets[0] = 7
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || !got.Applied {
		t.Fatal(got, err)
	}
	if err := m.Stores.Refill(0, &c.Systems.Damage); err != nil {
		t.Fatal(err)
	}
	if c.Systems.DamageMasks[0] != 0x81 || c.Systems.Damage[1] != 2 || c.Systems.Damage[2] != 1 || c.Systems.EngineTargets[0] != 7 || m.Stores.Ships[0].Flags&0xf8 != 0x18 || m.Stores.Ships[0].Stations[1].Ammunition != 0 {
		t.Fatal("source refill became full repair or discarded carried cargo")
	}
}

func TestPlayerDamageEngineFuelAndCrackConsumers(t *testing.T) {
	m, c := damageFixture(t, 3, 77)
	c.Systems.Damage[2] = 1
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.ReturnCode != 12 || c.Systems.EngineTargets[0] != 0 || c.Systems.Damage[9] != 1 {
		t.Fatal("single engine specialcode12", got, err)
	}
	m, c = damageFixture(t, 2, 95)
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.SourceStatus != 0 || m.Stores.Ships[0].Fuel != 98 || c.Systems.Fuel[0] != 98 {
		t.Fatal("SHIP17 lowbyte AND FE", got, err)
	}
	c.Random = func() uint16 { return 95 }
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.SourceStatus != 0x10 || got.Countdown != 4 || len(got.Audio) != 0 || !*c.SpeechEnabled {
		t.Fatal("fuel critical status/countdown blocks609 speech gate", got, err)
	}
	m, c = damageFixture(t, 2, 190, 1, 2)
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || *c.Overlay != [2][2]int16{{33, 42}, {}} || c.Systems.Damage[8] != 1 || c.Systems.EngineTargets != [2]int16{15, 15} {
		t.Fatal("608 owns crack coordinates, not engine oscillation", got, err)
	}
	q := []uint16{190, 240, 31}
	c.Random = func() uint16 { w := q[0]; q = q[1:]; return w }
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || *c.Overlay != [2][2]int16{{33, 42}, {272, 71}} || c.Systems.Damage[8] != 2 || c.Systems.Damage[9] != 0 {
		t.Fatal("second retained crack and excluded critical-health counter", got, err)
	}
	if m.Stores.Damage[0] != c.Systems.Damage {
		t.Fatal("service/current stores damage ownership diverged")
	}
}

func TestPlayerDamageWingMassAndSourceRow3Boundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selectR  uint16
		mode     byte
		quantity int16
		flags    uint16
		massLoss int16
	}{{"port-mode2", 51, 2, 11, 0, 40}, {"starboard-mode1", 146, 1, 11, 0, 40}, {"gun-burst-unsigned-div20", 51, 2, 55, 1, 13}} {
		t.Run(tc.name, func(t *testing.T) {
			m, c := damageFixture(t, 2, tc.selectR)
			w := RuntimeWeapon{CompactID: 2}
			binary.LittleEndian.PutUint16(w.Raw[:], tc.flags)
			binary.LittleEndian.PutUint16(w.Raw[11:], 3)
			binary.LittleEndian.PutUint16(w.Raw[13:], 7)
			m.Stores.Definitions = []RuntimeWeapon{w}
			m.Stores.Ships[0].Stations[1] = RuntimeStation{CompactID: 2, Ammunition: tc.quantity, Mode: tc.mode}
			got, err := m.ApplyAcceptedPlayerHit(c)
			if err != nil || !got.Applied || *c.MassTerm != 8000-tc.massLoss || m.Stores.Ships[0].Stations[1].Ammunition != 0 {
				t.Fatal("wing/mass arithmetic", got, err, *c.MassTerm)
			}
		})
	}
	m, c := damageFixture(t, 2, 51)
	m.Stores.Definitions = []RuntimeWeapon{{CompactID: 2}}
	m.Stores.Ships[0].Stations[1] = RuntimeStation{CompactID: 2, Ammunition: 19, Mode: 3}
	before := *c.Systems
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.Applied || !strings.Contains(got.Limit, "SHIP row3") || !reflect.DeepEqual(*c.Systems, before) || *c.MassTerm != 8000 {
		t.Fatal("unknown retained row silently invented or partly applied", got, err)
	}
	// The fresh writer's explicit padding closure is distinct from guessing an
	// unknown imported row. It emits a zeroed row3 without creating a follower.
	m.Stores.FreshShipPadding, c.FreshShipPadding = true, true
	c.Random = func() uint16 { return 51 }
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || !got.Applied || len(m.Stores.Ships) != 1 || m.Stores.Ships[0].Stations[1].Ammunition != 19 || *c.MassTerm != 8000 || c.Systems.Damage[1] != 2 {
		t.Fatal("fresh source padding was treated as retained memory or fake follower", got, err)
	}
	c.Systems.Damage[1] = 0
	m.Stores.Damage[0][1] = 0
	// Labelled established four-row supplied case exercises the literal CX3
	// source quirk. The selected player's paired quantity stays19; SHIP3 halves17→8.
	m.Stores.Ships = append(m.Stores.Ships, RuntimeAircraft{}, RuntimeAircraft{}, RuntimeAircraft{Stations: [4]RuntimeStation{{}, {Mode: 3, Ammunition: 17}}})
	c.Random = func() uint16 { return 51 }
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || !got.Applied || m.Stores.Ships[0].Stations[1].Ammunition != 19 || m.Stores.Ships[3].Stations[1].Ammunition != 8 || *c.MassTerm != 8000 || c.Systems.Damage[1] != 2 {
		t.Fatal("source CX alias replaced with victim half", got, err)
	}
}

func TestPlayerDamageFreshPaddingProducerPrepared(t *testing.T) {
	a := missionReadinessAssets(t)
	m, err := GenerateMission(a.Generation, MissionInputs{MissionType: MissionSingle, Theatre: TheatrePersianGulf, Rank: RankCaptain, PilotName: "Padding source", Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := InitializeReadinessRow(a, m, 0)
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildRuntimeStores(a, m, [][36]byte{r})
	if err != nil {
		t.Fatal(err)
	}
	if !s.FreshShipPadding || len(s.Ships) != 1 || len(s.Blueprint) != 1 {
		t.Fatal("fresh writer provenance missing or inactive actors fabricated")
	}
	if (RuntimeStores{}).FreshShipPadding {
		t.Fatal("generic supplied/imported carrier claimed source fresh padding")
	}
}

func TestPlayerDamageNativeLifecycleAndDegenerateRNGAreBounded(t *testing.T) {
	m, c := damageFixture(t, 2)
	m.Sortie = &SortieState{Result: &TransientSortieResult{Reason: "base", SourceStatus: 0x80}}
	c.Random = func() uint16 { t.Fatal("post-termination damage consumed RNG"); return 0 }
	got, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.Applied || got.Suppressed != "frozen sortie result" || m.Sortie.Result.SourceStatus != 0x80 {
		t.Fatal("stale damage changed frozen facts", got, err)
	}
	m, c = damageFixture(t, 2)
	c.Systems.DamageMasks[0] = 2
	c.Random = func() uint16 { return 0 } // Explicitly degenerate supplied stream, not a normal hit witness.
	got, err = m.ApplyAcceptedPlayerHit(c)
	if err != nil || got.Applied || !strings.Contains(got.Limit, "65536") || got.RNGCalls != 65536 || c.Systems.DamageMasks[0] != 2 {
		t.Fatal("unreachable reroll never returned an explicit native limit", got, err)
	}
}

func TestFuelHitCausingMultisystemPowerLossDoesNotCreateDestruction(t *testing.T) {
	m, c := damageFixture(t, 2, 95)
	c.ForcedLanding = true
	c.Systems.Damage = [10]byte{1, 1, 1, 1, 1}
	result, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || c.Systems.Damage[3] != 2 || c.Systems.Damage[9] != 1 || result.SourceStatus != 0 || result.Countdown != 0 {
		t.Fatal(result, err)
	}
}
