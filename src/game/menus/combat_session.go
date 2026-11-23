package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
	"log/slog"
)

func (p *RenderingPreview) combatInput(df byte, phase uint16, random func() uint16) combat.Input {
	r := p.Flight
	var engagement func(actor, target int)
	if p.wingmanEngagementCompression {
		engagement = func(actor, target int) {
			p.limitFlightCompressionCause("wingman-accepted-release", actor, target)
		}
	}
	var hostileTarget func(actor, target int)
	if p.stationaryEnemyMechanics {
		hostileTarget = p.hostileTargetSelected
	}
	return combat.Input{FollowerEngagement: engagement, TranslationHistory0: &r.State.History0, ContactCandidates: &r.Avionics.Candidates, WeatherRemap: p.Renderer.Assets.WeatherRemap, HoldHeightTarget: &r.State.HeightTarget, ObserveSupport: func(actor, target int) (bool, error) {
		return r.Avionics.ObserveSupport(avionics.Input{Entities: p.contactEntities(), World: p.Initialized.World, Assets: p.Renderer.Assets}, actor, target)
	}, PlayerContactHeight: r.State.ContactHeight, DF: df, Phase: phase, LowLimit: r.LowLimit, SelectedStation: p.Initialized.SelectedStation,
		Salvo: r.RocketSalvo, Target: r.Avionics.Target, TargetAvailable: p.Cockpit.TargetSymbol == 1,
		Contacts: r.Avionics.Contacts, ContactState: &r.Avionics.Contacts, PlayerIndicatedSpeed: r.State.IndicatedSpeed, PlayerSpeed: r.State.Forward, GearFlags: r.SystemFlags,
		Damage: r.Systems.Damage, AudioEnabled: true, SpeechEnabled: r.SpeechEnabled, Random: random, Incoming: p.incomingContext(), HostileTarget: hostileTarget}
}

func (p *RenderingPreview) advanceCombat(in combat.Input) error {
	r := p.Flight
	// Manual selection may have changed AB9 after the held-fire arm. B0B is
	// the previous logical display result, not a newly invented lock boolean.
	in.Target = r.Avionics.Target
	observed, err := r.Combat.AdvanceFollowers(in, &r.Systems)
	if err != nil {
		return err
	}
	for _, row := range observed {
		if row < 0 || row >= len(r.Avionics.Contacts) {
			return fmt.Errorf("wreck contact row%d invalid", row)
		}
		c, e := &r.Avionics.Contacts[row], p.Initialized.Entities[row]
		c.Flags, c.Kind, c.H0, c.H1 = (c.Flags&0xf9)|1, e.Kind, e.Pose.H0, e.Pose.H1
	}
	if err = r.Combat.Step(in); err != nil {
		return err
	}
	p.collectCombatEvents()
	p.advanceCombatAppearance(in.Phase)
	return nil
}

func (p *RenderingPreview) collectCombatEvents() {
	r := p.Flight
	for _, e := range r.Combat.DrainEvents() {
		switch e.Kind {
		case "sound":
			r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: e.Command, firingAircraft: p.firingAircraft(e.Slot, e.Command)})
		case "mass":
			r.MassTerm -= e.Value //57b4 subtracts the definition's word once per allocation.
		case "salvo":
			r.RocketSalvo = e.Value         //5886 lowers AC1 until remaining ammo >=2*AC1.
			p.Cockpit.RocketSalvo = e.Value // The later HUD phase sees the new word this frame.
		case "shake":
			r.State.Shake = e.Value //8c68 writes ACD after this update's player prefix.
		case "launch":
			if e.Slot >= 0 && e.Slot < len(r.projectileGeneration) {
				r.projectileGeneration[e.Slot]++
			}
			if p.stationaryEnemyMechanics && e.Slot >= 20 && diagnostics.Enabled(slog.LevelInfo) {
				diagnostics.Info("combat", "Stationary hostile released projectile", "actor", e.Actor, "target", e.Target, "slot", e.Slot, "definition", e.Value, "phase", r.ElapsedDA)
			}
		case "hostile-miss", "hostile-impact", "hostile-defended", "hostile-guidance-defeated", "npc-damage":
			if diagnostics.Enabled(slog.LevelDebug) {
				diagnostics.Debug("combat", "Hostile projectile outcome", "outcome", e.Kind, "actor", e.Actor, "target", e.Target, "slot", e.Slot, "cause", e.Source, "phase", r.ElapsedDA)
			}
		case "objective-contact-kind":
			if e.Target >= 0 && e.Target < len(r.Avionics.Contacts) {
				r.Avionics.Contacts[e.Target].Kind = byte(e.Value)
			}
		case "contact-stale":
			if e.Target >= 0 && e.Target < len(r.Avionics.Contacts) {
				r.Avionics.Contacts[e.Target].Flags = r.Avionics.Contacts[e.Target].Flags&0xf9 | 1
			}
		case "notification":
			argument := int16(0)
			if r.Combat.MissionActorsEnabled() && e.Notification == 2 {
				argument = int16(e.Target)
			}
			r.Notifications.Queue(int16(e.Notification), int16(e.Actor), argument)
		}
		r.CombatEvents = append(r.CombatEvents, e)
		if len(r.CombatEvents) > 256 {
			r.CombatEvents = r.CombatEvents[len(r.CombatEvents)-256:]
		}
	}
}

func (p *RenderingPreview) advanceCombatObjectives(in combat.Input) {
	r := p.Flight
	if p.ResultTargets == nil {
		// Legacy supplied fixtures lack a full TARGET carrier. Retain their
		// existing bounded current-count path, never fabricate an alternate.
		for _, e := range p.Initialized.Entities {
			if e.Object.Raw[0]&0x40 != 0 && e.Kind == 0x1c {
				in.SecondaryCountReady = true
				break
			}
		}
	}

	r.Combat.AdvanceObjectives(in)
	if p.ResultTargets != nil {
		binary.LittleEndian.PutUint16(p.ResultTargets.Records[2][32:], uint16(r.Combat.Primary.DisplayKind))
		if p.ResultTargets.ProgressFailure == "" {
			binary.LittleEndian.PutUint16(p.ResultTargets.Records[3][32:], uint16(r.Combat.Secondary.DisplayKind))
		}
	}
	p.collectCombatEvents()
	if r.Combat.Primary.Achieved && r.Combat.Primary.DisplayKind == 0 {
		p.Cockpit.Objectives[0].Status = 0
	}
	if r.Combat.Secondary.Achieved && r.Combat.Secondary.DisplayKind == 0 {
		p.Cockpit.Objectives[1].Status = 0
	}
	p.publishObjectiveDisplays()
	p.filterSceneInstances()
	p.publishMapRoutes()
}

func (p *RenderingPreview) advanceCombatNotifications(e0 bool) {
	r := p.Flight
	for _, command := range r.Notifications.Advance(e0, true) {
		r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: command})
	}
	// M's admitted producers are player-row0 achievements. No NPC/radio name
	// or leader aliases are synthesized for unimplemented tactical messages.
	in := &cockpit.NotificationInputs{Rows: r.Notifications.Display, Player: byte(p.Initialized.PlayerRow), LeaderA: p.gpsLeaderSelector(4), LeaderB: p.gpsLeaderSelector(3)}
	for _, e := range p.Initialized.Entities {
		in.ObjectNames = append(in.ObjectNames, stringTrimZero(e.Object.Raw[15:27]))
	}
	if p.ResultTargets != nil {
		in.SecondaryName = stringTrimZero(p.ResultTargets.Records[3][:32])
	}
	p.Cockpit.Notifications = in
}

// This is the logical counterpart of8899/99ac's shared source streams. Every
// registered explosion sees the final stream, including retained Y words in
// the participation3 branch. Extra presentation draws cannot mutate it.
func (p *RenderingPreview) advanceCombatAppearance(phase uint16) {
	r, c := p.Flight, p.Renderer.Assets.Combat
	if r == nil || r.Combat == nil || c == nil {
		return
	}
	p.retainReservedRescueInstances()
	if p.p4Mechanics {
		p.advanceDeathHistory(phase)
	}
	if r.deathAngles == nil {
		r.deathAngles = map[int][3]uint16{}
	}
	if len(r.explosionVertices) == 0 {
		r.explosionVertices = append([]world.Vertex(nil), p.Renderer.Assets.Geometries[c.Explosion].Vertices...)
	}
	for row, e := range p.Initialized.Entities {
		if e.Participation != 2 && e.Participation != 3 {
			delete(r.deathAngles, row)
			continue
		}
		a, ok := r.deathAngles[row]
		if !ok {
			a = e.Pose.Angles
		}
		if e.Participation == 2 {
			a[0] += 0x100
			a[1] += 0x400
			a[2] += 0x1000
		}
		r.deathAngles[row] = a
		for i, base := range c.ExplosionBase {
			d := int16((phase&15)<<3) + 64
			expand := func(v int16) int16 {
				if v < 0 {
					return v - d
				}
				return v + d
			}
			if e.Participation == 2 {
				for j := range base {
					r.explosionVertices[i][j] = expand(base[j])
				}
			} else {
				//8a63 overwrites X from the retained current Y. SI advances by6,
				//the independent BP-6 counter by3; neither is a phase frequency.
				x := r.explosionVertices[i][1]
				if phase&0x10 != 0 {
					x += int16(2 + 6*i)
				} else {
					x -= int16(1 + 3*i)
				}
				r.explosionVertices[i][0], r.explosionVertices[i][2] = x, expand(base[2])
			}
		}
	}
	for slot, q := range r.Combat.Slots {
		if q.Lifetime == 0 {
			continue
		}
		a := &r.projectileAngles[slot]
		a[0], a[1] = q.Pose.Angles[0], q.Pose.Angles[1]
		if q.Flags&3 == 0 {
			a[2] += 0x1000
		} else {
			a[2] = 0
		}
	}
	p.Scene.CombatPhase = phase
	p.Scene.ExplosionVertices = append(p.Scene.ExplosionVertices[:0], r.explosionVertices...)
	// Destruction replaces the current application header, not Object.Raw.
	for i := range p.instances {
		for _, e := range p.Initialized.Entities {
			if p.instances[i].ID != e.ID {
				continue
			}
			p.instances[i].ModelID = e.ModelID
			p.instances[i].MutableModel = p.Initialized.SubmarineModels[e.RuntimeRow]
			p.instances[i].Pose = render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}
			p.instances[i].Angles = e.Pose.Angles
			if a, ok := r.deathAngles[e.RuntimeRow]; ok {
				p.instances[i].Angles = a
			}
			// Both0afe replacement-loader callers pass flags0/actorFFFF;11f0's
			// common cache tail installs default13bf, including cache hits.
			if e.Kind == 0x1c || e.Kind == 0x1d {
				p.instances[i].Callback = 0
				p.instances[i].GraphChild = nil
				p.instances[i].Oriented = false
			}
			break
		}
	}
}

// Source8899/8ad8 owns one global eight-entry10-byte pose history. Publication
// snapshots world registrations before ABD's decrement; target-MFD reads the
// resulting counter. Multiple P2 rows retain the source ascending overwrite order.
func (p *RenderingPreview) advanceDeathHistory(phase uint16) {
	r := p.Flight
	r.deathHistoryWorld = r.deathHistoryWorld[:0]
	c := p.Renderer.Assets.Combat
	if c == nil || len(c.DeathHistory) != 8 {
		return
	}
	for _, e := range p.Initialized.Entities {
		if e.Kind == 0xff || e.Kind&0x40 != 0 || e.Participation != 2 {
			continue
		}
		for i := int(r.deathHistoryCount); i < 8; i++ {
			r.deathHistory[i] = render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}
		}
		if r.deathHistoryCount == 0 {
			r.deathHistoryCount = 8
		}
	}
	if r.deathHistoryCount == 0 {
		return
	}
	for i := int(r.deathHistoryCount); i < 8; i++ {
		q := &r.deathHistory[i]
		q.Height += uint16((8-i)*128) + ((phase-uint16(i))&3)*8
		shift := int32(int16(uint16(4) << uint(8-i)))
		if i&1 == 0 {
			shift = -shift
		}
		q.H0 += uint32(shift)
		q.H1 += uint32(shift)
		r.deathHistoryWorld = append(r.deathHistoryWorld, render3d.Instance{ID: fmt.Sprintf("death-history-%d", i), GeometryID: c.DeathHistory[i], Pose: *q})
	}
	r.deathHistoryCount--
}

func (p *RenderingPreview) p4AppearanceCheckpoint() any {
	if p == nil || p.Flight == nil {
		return nil
	}
	return struct {
		Count uint16
		Poses [8]render3d.Pose
	}{p.Flight.deathHistoryCount, p.Flight.deathHistory}
}

// Reserved source rows5..8 have real headers even while their initial P0 hides
// them. Activation changes their ordinary registration eligibility, not their
// identity into the damaged helicopter's header. Keep metadata after pickup so
// kind41 can be filtered without duplicating/reconstructing the actor.
func (p *RenderingPreview) retainReservedRescueInstances() {
	m := p.Initialized
	if m == nil || len(m.FlightRows) != 5 || len(m.Entities) < 9 {
		return
	}
	for actor := 1; actor < 5; actor++ {
		row := actor + 4
		e := m.Entities[row]
		if m.FlightRows[actor] != actor || e.RuntimeRow != row || e.SourceRow != row || e.Object.Raw[10] != 0x1b || !e.ControlCarrierOnly {
			continue // No invented aliases for imported or unclosed actor carriers.
		}
		present := false
		for _, in := range p.instances {
			if in.ID == e.ID {
				present = true
				break
			}
		}
		if present {
			continue
		}
		if e.Participation == 0 || e.Kind == 0xff || e.Kind&0x40 != 0 {
			continue // Register on actual activation; retain the header thereafter.
		}
		flags := binary.LittleEndian.Uint16(e.Object.Raw[:])
		callback := uint8(flags >> 8)
		if callback > 7 || e.ModelID < 0 || e.ModelID >= len(p.Renderer.Assets.Models) {
			continue // These source-loaded headers are validated by initialization.
		}
		p.instances = append(p.instances, render3d.Instance{ID: e.ID, ModelID: e.ModelID,
			Pose: render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}, Angles: e.Pose.Angles,
			Callback: callback, Oriented: flags&8 != 0})
	}
}

func (p *RenderingPreview) combatInstances() []render3d.Instance {
	r := p.Flight
	if r == nil || r.Combat == nil || p.Renderer.Assets.Combat == nil {
		return nil
	}
	c := p.Renderer.Assets.Combat
	var out []render3d.Instance
	for slot, q := range r.Combat.Slots {
		if q.Lifetime == 0 && q.Packed == 0 {
			continue
		}
		v := render3d.Instance{ID: fmt.Sprintf("projectile-%d-%d", slot, r.projectileGeneration[slot]),
			Pose: render3d.Pose{H0: q.Pose.H0, H1: q.Pose.H1, Height: q.Pose.Height}}
		if q.Lifetime == 0 {
			v.ID += "-impact"
			v.Impact = true
		} else {
			v.GeometryID = c.Templates[q.Template]
			v.Oriented, v.Angles = true, r.projectileAngles[slot]
		}
		out = append(out, v)
	}
	return out
}

// Preserve raw26 bytes for deterministic diagnostic state without exporting
// renderer-only caches as authoritative flight state.
func (p *RenderingPreview) combatRaw() []byte {
	if p.Flight == nil || p.Flight.Combat == nil {
		return nil
	}
	r := p.Flight.Combat
	out := make([]byte, 0, combat.SlotCount*26+4)
	for _, q := range r.Slots {
		b := q.Raw()
		out = append(out, b[:]...)
	}
	return binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(out, r.NextSlot), r.LastSlot)
}
