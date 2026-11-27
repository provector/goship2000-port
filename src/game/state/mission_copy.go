package state

import (
	"maps"
	"slices"

	"gunship2000-assets/game/world"
)

// CopyMissionState copies the mutable logical mission. Model copies are also
// returned by original identity so scene instances can retain their bindings.
func (m *InitializedMission) CopyMissionState() (*InitializedMission, map[*world.MutableModel]*world.MutableModel) {
	n := *m
	models := make(map[*world.MutableModel]*world.MutableModel)
	n.SubmarineModels = maps.Clone(m.SubmarineModels)
	for k, model := range m.SubmarineModels {
		if model == nil {
			continue
		}
		if models[model] == nil {
			v := *model
			v.Vertices = slices.Clone(model.Vertices)
			v.Children = slices.Clone(model.Children)
			models[model] = &v
		}
		n.SubmarineModels[k] = models[model]
	}
	if m.EnemyRules != nil {
		rules := *m.EnemyRules
		n.EnemyRules = &rules
	}
	if m.Support != nil {
		s := *m.Support
		s.Configuration.Rows = slices.Clone(s.Configuration.Rows)
		s.Configuration.RetainedRows = slices.Clone(s.Configuration.RetainedRows)
		n.Support = &s
	}
	if m.PlayerFlight != nil {
		f := *m.PlayerFlight
		if f.Terminal != nil {
			t := *f.Terminal
			f.Terminal = &t
		}
		n.PlayerFlight = &f
	}
	if m.Stores != nil {
		s := *m.Stores
		if s.WeaponTable != nil {
			table := *s.WeaponTable
			s.WeaponTable = &table
		}
		s.Definitions = slices.Clone(s.Definitions)
		s.Damage = slices.Clone(s.Damage)
		s.Ships = slices.Clone(s.Ships)
		s.Blueprint = slices.Clone(s.Blueprint)
		skills := map[*byte]*byte{}
		for _, rows := range [][]RuntimeAircraft{s.Ships, s.Blueprint} {
			for i := range rows {
				p := rows[i].Proficiency
				if p == nil {
					continue
				}
				if skills[p] == nil {
					v := *p
					skills[p] = &v
				}
				rows[i].Proficiency = skills[p]
			}
		}
		if s.NPCDamage != nil {
			d := *s.NPCDamage
			d.Masks = slices.Clone(d.Masks)
			s.NPCDamage = &d
		}
		n.Stores = &s
	}
	if m.Sortie != nil {
		s := *m.Sortie
		if s.Result != nil {
			v := *s.Result
			s.Result = &v
		}
		n.Sortie = &s
	}
	n.CrewPerformance = slices.Clone(m.CrewPerformance)
	n.Targets = CloneMissionTargets(m.Targets)
	n.ActorFlags = slices.Clone(m.ActorFlags)
	n.Rows = slices.Clone(m.Rows)
	n.Entities = slices.Clone(m.Entities)
	n.Objectives = maps.Clone(m.Objectives)
	n.FlightRows = slices.Clone(m.FlightRows)
	n.Flight = slices.Clone(m.Flight)
	for i := range n.Flight {
		n.Flight[i].AwardCounts = slices.Clone(n.Flight[i].AwardCounts)
	}
	n.Aircraft = slices.Clone(m.Aircraft)
	n.Paths = slices.Clone(m.Paths)
	n.Draws = slices.Clone(m.Draws)
	n.InitialFuelPercentBySlot = slices.Clone(m.InitialFuelPercentBySlot)
	n.Status = slices.Clone(m.Status)
	n.Offsets = maps.Clone(m.Offsets)
	for k, v := range m.Offsets {
		n.Offsets[k] = slices.Clone(v)
	}
	return &n, models
}
