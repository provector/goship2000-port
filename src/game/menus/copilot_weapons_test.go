package menus

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Supplied predicate fixture: real prepared integer tables, a flat world and
// explicit weapon/target operands. This is separate from ordinary sortie proof.
func weaponFixture(t *testing.T, flags uint16) (*RenderingPreview, combat.Input, *int) {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_WORLD")
	if dir == "" {
		t.Skip("prepared world tables required")
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var a world.Bundle
	if err = json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	if a.Sources["GS2.GS2"] != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported source angle tables", a.Sources)
	}
	a.Worlds = map[string]*world.World{"flat": {Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}}
	for i := range a.Worlds["flat"].Cells {
		a.Worlds["flat"].Cells[i].Geometry = "flat"
	}
	a.Geometries = map[string]*world.Geometry{"flat": {Heights: []world.HeightRegion{{Points: [][2]int16{{-4096, -4096}, {4096, -4096}, {4096, 4096}, {-4096, 4096}, {-4096, -4096}}, Plane: world.Plane{0, 1, 0, 0}}}}}
	w := gamestate.RuntimeWeapon{CompactID: 2, RawWSHDID: 4}
	binary.LittleEndian.PutUint16(w.Raw[:], flags)
	binary.LittleEndian.PutUint16(w.Raw[9:], 100)
	binary.LittleEndian.PutUint16(w.Raw[11:], 7)
	w.Raw[4], w.Raw[5], w.Raw[6], w.Raw[7] = 100, 100, 100, 30
	m := &gamestate.InitializedMission{Switches: 0x18, World: "flat", PlayerRow: 0, SelectedStation: 1,
		Stores: &gamestate.RuntimeStores{Definitions: []gamestate.RuntimeWeapon{w}, Ships: []gamestate.RuntimeAircraft{{}}}}
	for i := range m.Stores.Ships[0].Stations {
		m.Stores.Ships[0].Stations[i] = gamestate.RuntimeStation{Present: true, CompactID: 2, RawWSHDID: 4, Ammunition: 200, Mode: 1}
	}
	m.Entities = []gamestate.InitializedEntity{
		{RuntimeRow: 0, Kind: 0xff, Participation: 0, Pose: gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 512}},
		{RuntimeRow: 1, Kind: 3, Participation: 1, Pose: gamestate.InitializedPose{H0: 0x40000, H1: 0x40000 + 16000, Height: 512}},
	}
	s, err := combat.New(m, &a, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &RenderingPreview{Initialized: m, Renderer: &render3d.Renderer{Assets: &a}, ViewControls: &render3d.CameraController{},
		Flight: &FlightRuntime{Combat: s, State: &flight.State{Pose: flight.Pose{H0: 0x40000, H1: 0x40000, Height: 512}}, Avionics: avionics.New(), RocketSalvo: 1}}
	p.Flight.Avionics.Target = 2
	p.Flight.Avionics.Contacts[1] = avionics.Contact{Kind: 3}
	draws := new(int)
	in := combat.Input{DF: 2, Phase: 8, E0: true, SelectedStation: 1, Salvo: 1, Target: 2, TargetAvailable: true,
		Random: func() uint16 { *draws++; return 0 }}
	return p, in, draws
}

func copilotWeaponsLaunches(p *RenderingPreview) (n int) {
	for _, e := range p.Flight.CombatEvents {
		if e.Kind == "launch" {
			n++
		}
	}
	return
}

func TestCopilotWeaponCallerPredicates(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RenderingPreview, *combat.Input)
		launch bool
	}{
		{"weapons-cm", func(*RenderingPreview, *combat.Input) {}, true},
		{"weapons-alone", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Switches = 0x10 }, true},
		{"cm-only", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Switches = 8 }, false},
		{"none", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Switches = 0 }, false},
		{"not-e0", func(_ *RenderingPreview, in *combat.Input) { in.E0 = false }, false},
		{"wrong-phase", func(_ *RenderingPreview, in *combat.Input) { in.Phase = 10 }, false},
		{"missing-target", func(_ *RenderingPreview, in *combat.Input) { in.Target = 0 }, false},
		{"invalid-aim", func(_ *RenderingPreview, in *combat.Input) { in.TargetAvailable = false }, false},
		{"kind10", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Entities[1].Kind = 0x13 }, false},
		{"kind20", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Entities[1].Kind = 0x23 }, false},
		{"inactive-target", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Entities[1].Participation = 0 }, false},
		{"p2-target", func(p *RenderingPreview, _ *combat.Input) { p.Initialized.Entities[1].Participation = 2 }, false},
		{"weapon-ignores-defense-damage40", func(p *RenderingPreview, _ *combat.Input) { p.Flight.Systems.DamageMasks[0] = 0x40 }, true},
		{"current-ring-busy", func(p *RenderingPreview, _ *combat.Input) { p.Flight.Combat.Slots[0].Lifetime = 1 }, false},
		{"current-ring-tail-free", func(p *RenderingPreview, _ *combat.Input) { p.Flight.Combat.Slots[0].Packed = 4 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, in, draws := weaponFixture(t, 0x0e10)
			tc.change(p, &in)
			if err := p.advanceCopilotWeapons(in); err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.launch {
				want = 1
			}
			if copilotWeaponsLaunches(p) != want || *draws != 0 || p.Initialized.SelectedStation != 1 {
				t.Fatal("caller result", copilotWeaponsLaunches(p), want, *draws)
			}
		})
	}
}

func TestCopilotWeaponPoliciesAndSalvoPublication(t *testing.T) {
	for _, enemyDisabled := range []bool{false, true} {
		for _, wingmanDisabled := range []bool{false, true} {
			p, in, draws := weaponFixture(t, 0x1e21)
			var definitions [16][8]byte
			for i := range definitions {
				definitions[i][0] = byte(i)
			}
			if err := p.Flight.Combat.ConfigureStationaryEnemyAI(definitions, enemyDisabled); err != nil {
				t.Fatal(err)
			}
			p.Flight.Combat.SetWingmenAutonomyEnabled(!wingmanDisabled)
			p.Initialized.Stores.Ships[0].Stations[1].Mode = 3
			p.Initialized.Stores.Ships[0].Stations[1].Ammunition = 15
			in.Salvo = 4
			if err := p.advanceCopilotWeapons(in); err != nil {
				t.Fatal(err)
			}
			if copilotWeaponsLaunches(p) != 2 || *draws != 0 || p.Initialized.SelectedStation != 1 || p.Initialized.Stores.Ships[0].Stations[1].Ammunition != 7 || p.Flight.RocketSalvo != 2 || p.Cockpit.RocketSalvo != 2 || p.Flight.MassTerm != -14 {
				t.Fatal("AI-independent paired salvo", enemyDisabled, wingmanDisabled, p.Flight.RocketSalvo, p.Cockpit.RocketSalvo)
			}
		}
	}
}

func TestCopilotWeaponUnsupportedDomainsFailWithoutMutation(t *testing.T) {
	for _, name := range []string{"target", "ring", "station"} {
		p, in, _ := weaponFixture(t, 0x0e10)
		switch name {
		case "target":
			in.Target = 81
		case "ring":
			p.Flight.Combat.NextSlot = 16
		case "station":
			in.SelectedStation = 4
		}
		before := p.Flight.Combat.FilmState()
		if err := p.advanceCopilotWeapons(in); err == nil || !reflect.DeepEqual(before, p.Flight.Combat.FilmState()) {
			t.Fatal("unsupported domain", name, err)
		}
	}
}

func TestCopilotWeaponExistingFireOwner(t *testing.T) {
	for _, name := range []string{"empty", "absent", "next-busy", "cannon-destroyed", "cannon-random-denied", "cannon-random-allowed", "closed-bay", "launcher-damage"} {
		t.Run(name, func(t *testing.T) {
			flags := uint16(0x0e10)
			if name == "launcher-damage" {
				flags = 0x1e21
			}
			if name == "cannon-destroyed" || name == "cannon-random-denied" || name == "cannon-random-allowed" {
				flags = 0x0e02
			}
			p, in, draws := weaponFixture(t, flags)
			station := &p.Initialized.Stores.Ships[0].Stations[1]
			wantLaunch, wantNext, wantDraws := 0, uint16(0), 0
			switch name {
			case "empty":
				station.Ammunition = 0
			case "absent":
				station.Present = false
			case "next-busy":
				p.Flight.Combat.Slots[1].Lifetime = 80
				wantNext = 1
			case "closed-bay":
				in.GearFlags = 1
			case "cannon-destroyed":
				in.SelectedStation = 0
				in.Damage[4] = 2
				wantNext = 1
			case "cannon-random-denied":
				in.SelectedStation = 0
				in.Damage[4] = 1
				wantNext, wantDraws = 1, 1
				in.Random = func() uint16 { *draws++; return 1 }
			case "cannon-random-allowed":
				in.SelectedStation = 0
				in.Damage[4] = 1
				wantLaunch, wantNext, wantDraws = 1, 1, 1
			case "launcher-damage":
				station.Mode = 3
				in.Damage[1] = 2
				wantLaunch, wantNext = 1, 1
			}
			before := p.Initialized.Stores.Ships[0].Stations[in.SelectedStation].Ammunition
			if err := p.advanceCopilotWeapons(in); err != nil {
				t.Fatal(err)
			}
			if copilotWeaponsLaunches(p) != wantLaunch || p.Flight.Combat.NextSlot != wantNext || *draws != wantDraws {
				t.Fatal(copilotWeaponsLaunches(p), p.Flight.Combat.NextSlot, *draws)
			}
			used := before - p.Initialized.Stores.Ships[0].Stations[in.SelectedStation].Ammunition
			wantUsed := int16(wantLaunch)
			if in.SelectedStation == 0 {
				wantUsed *= 20
			}
			if used != wantUsed {
				t.Fatal("ammunition", used, wantUsed)
			}
		})
	}
}

func TestCopilotManualNearPhaseAndCooldown(t *testing.T) {
	for _, flags := range []uint16{0x0e10, 0x1e21, 0x0e02} {
		p, in, draws := weaponFixture(t, flags)
		if err := p.Flight.Combat.Fire(in); err != nil {
			t.Fatal(err)
		}
		p.collectCombatEvents()
		before := p.Flight.Combat.FilmState()
		ammo := p.Initialized.Stores.Ships[0].Stations[1].Ammunition
		if err := p.advanceCopilotWeapons(in); err != nil {
			t.Fatal(err)
		}
		if copilotWeaponsLaunches(p) != 1 || *draws != 0 || !reflect.DeepEqual(before, p.Flight.Combat.FilmState()) || p.Initialized.Stores.Ships[0].Stations[1].Ammunition != ammo {
			t.Fatal("late arm duplicated near manual release")
		}
		// Distinguish the caller's current-slot gate from559E's separate previous
		// slot family cooldown, including strict equality acceptance.
		threshold := byte(131)
		if flags&3 == 0 {
			threshold = 123
		}
		if flags&3 == 1 {
			threshold = 127
		}
		p.Flight.Combat.NextSlot, p.Flight.Combat.LastSlot = 2, 1
		p.Flight.Combat.Slots[1].Lifetime = threshold + 1
		if err := p.advanceCopilotWeapons(in); err != nil {
			t.Fatal(err)
		}
		if p.Flight.Combat.NextSlot != 2 || copilotWeaponsLaunches(p) != 1 {
			t.Fatal("cooldown did not inhibit")
		}
		p.Flight.Combat.Slots[1].Lifetime = threshold
		if err := p.advanceCopilotWeapons(in); err != nil {
			t.Fatal(err)
		}
		if p.Flight.Combat.NextSlot != 3 || copilotWeaponsLaunches(p) != 2 {
			t.Fatal("cooldown equality should allow")
		}
	}
}

func TestCopilotSourceWeaponFamilyEligibility(t *testing.T) {
	// Independent expected operands from the retained raw18-definition matrix.
	// Lowbit1, rather than family number or a cannon label, owns the tight cone.
	for raw, flags := range []uint16{0x0288, 0x0288, 0x0c48, 0x0e20, 0x0e10, 0x0e18, 0x0e10, 0x1e21, 0x1e21, 0x1e21, 0x0c08, 0x0408, 0x0e01, 0x0e01, 0x0e01, 0x0e02, 0x0e02, 0x0e02} {
		p, in, draws := weaponFixture(t, flags)
		kind, attack := byte(3), uint16(0)
		if raw < 2 {
			kind = 1
		}
		if raw == 2 {
			attack = 0x2000
		}
		if raw == 11 {
			kind = 2
		}
		p.Initialized.Stores.Definitions[0].RawWSHDID = raw
		p.Flight.Avionics.Contacts[1] = avionics.Contact{Kind: kind, AttackFlags: attack}
		p.Initialized.Entities[1].Kind = kind
		p.publishTargeting()
		if p.Cockpit.TargetSymbol != 1 {
			t.Fatalf("raw%d centered eligible symbol%d", raw, p.Cockpit.TargetSymbol)
		}
		in.TargetAvailable, in.Contacts = p.Cockpit.TargetSymbol == 1, p.Flight.Avionics.Contacts
		if err := p.advanceCopilotWeapons(in); err != nil {
			t.Fatal(raw, err)
		}
		if copilotWeaponsLaunches(p) != 1 || *draws != 0 {
			t.Fatal("raw family release", raw, copilotWeaponsLaunches(p), *draws)
		}
		for _, tc := range []struct {
			x, y     int16
			distance uint16
			want     int
		}{
			{0, 0, 99, 1}, {0, 0, 100, 2}, {0, 0, 101, 2},
			{4, 4, 99, 1}, {-4, -4, 99, 1}, {5, 0, 99, 1}, {-5, 0, 99, 1}, {0, 5, 99, 1}, {0, -5, 99, 1},
			{95, 63, 99, 1}, {-95, -63, 99, 1}, {96, 0, 99, 3}, {-96, 0, 99, 3}, {0, 64, 99, 3}, {0, -64, 99, 3},
		} {
			want := tc.want
			if flags&1 != 0 && (tc.x >= 5 || tc.y >= 5 || tc.x <= -5 || tc.y <= -5) {
				want = 3
			}
			if got := avionics.TargetSymbol(true, tc.x, tc.y, tc.distance, 100, flags, kind, attack); got != want {
				t.Fatalf("raw%d flags%04x %+v =>%d want%d", raw, flags, tc, got, want)
			}
		}
		if raw < 2 && avionics.TargetSymbol(true, 0, 0, 99, 100, flags, 3, 0) != 3 {
			t.Fatal("IR-air admitted land", raw)
		}
		if raw == 2 && avionics.TargetSymbol(true, 0, 0, 99, 100, flags, 3, 0) != 3 {
			t.Fatal("radar admitted nonemitter")
		}
		if (raw == 10 || raw == 11) && avionics.TargetSymbol(true, 0, 0, 99, 100, flags, 1, attack) != 3 {
			t.Fatal("no air capability", raw)
		}
		if raw == 11 && avionics.TargetSymbol(true, 0, 0, 99, 100, flags, 3, 0) != 3 {
			t.Fatal("Penguin admitted land")
		}
	}
}

func TestCopilotActualAimRangeAngleAndMast(t *testing.T) {
	for _, name := range []string{"range", "angle", "behind", "mast-sees-body-ineligible", "mast-offscreen", "mast-body-eligible"} {
		t.Run(name, func(t *testing.T) {
			p, in, _ := weaponFixture(t, 0x0e10)
			target := &p.Initialized.Entities[1].Pose
			switch name {
			case "range":
				target.H1 += 50000
			case "angle":
				target.H0 += 16000
			case "behind":
				target.H1 = 0x40000 - 16000
			case "mast-sees-body-ineligible":
				target.H1 = 0x40000 - 16000
				p.ViewControls.Mode = 7
				p.ViewControls.Camera = render3d.Camera{Pose: render3d.Pose{H0: 0x40000, H1: 0x40000, Height: 512}, Angles: [3]uint16{0, 0x8000, 0}}
			case "mast-offscreen":
				p.ViewControls.Mode = 7
				p.ViewControls.Camera = render3d.Camera{Pose: render3d.Pose{H0: 0x40000, H1: 0x40000, Height: 512}, Angles: [3]uint16{0, 0x8000, 0}}
			case "mast-body-eligible":
				p.ViewControls.Mode = 7
				p.ViewControls.Camera = render3d.Camera{Pose: render3d.Pose{H0: 0x40000, H1: 0x40000, Height: 640}}
			}
			p.publishTargeting()
			in.TargetAvailable = p.Cockpit.TargetSymbol == 1
			if err := p.advanceCopilotWeapons(in); err != nil {
				t.Fatal(err)
			}
			want := 0
			if name == "mast-body-eligible" {
				want = 1
			}
			if copilotWeaponsLaunches(p) != want {
				t.Fatal("actual body/display eligibility", name, p.Cockpit.TargetSymbol, copilotWeaponsLaunches(p), want)
			}
		})
	}
}

func TestCopilotLateReleaseFirstVisitNextProjectilePass(t *testing.T) {
	p, in, _ := weaponFixture(t, 0x0e10)
	if err := p.Flight.Combat.Step(in); err != nil {
		t.Fatal(err)
	}
	if err := p.advanceCopilotWeapons(in); err != nil {
		t.Fatal(err)
	}
	born := p.Flight.Combat.Slots[1]
	if born.Lifetime != 131 || p.Flight.Combat.Updates != 1 || p.Flight.MassTerm != -7 || p.Flight.projectileGeneration[1] != 1 {
		t.Fatal("birth publication", born, p.Flight.MassTerm, p.Flight.projectileGeneration[1])
	}
	if err := p.Flight.Combat.Step(in); err != nil {
		t.Fatal(err)
	}
	if p.Flight.Combat.Slots[1].Lifetime != 130 || p.Flight.Combat.Updates != 2 {
		t.Fatal("not visited on next ascending pass")
	}
}
