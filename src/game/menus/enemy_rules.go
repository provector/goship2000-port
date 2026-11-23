package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

func freezePreparedEnemyRules(a *assetdata.MissionAssets, m *gamestate.MissionData, switches uint16, disabled bool) (*gamestate.EnemyRules, error) {
	if a == nil || a.Generation == nil || m == nil || m.SourceWorld == nil {
		return nil, fmt.Errorf("selected world enemy definition owner missing")
	}
	rows, err := a.Generation.EnemyDefinitionsForWorld(m.SourceWorld.WorldName)
	if err != nil {
		return nil, err
	}
	var raw [16][8]byte
	if len(rows) != len(raw) {
		return nil, fmt.Errorf("selected world needs16 enemy definitions")
	}
	for i := range raw {
		raw[i] = [8]byte(rows[i])
	}
	return gamestate.FreezeEnemyRules(m.World, raw, switches, disabled)
}

// Source A27E/A294 and A29F..A2B6 are target-selection effects, including
// waiting/no-shot visits. The radio or projectile launch is not their trigger.
func (p *RenderingPreview) hostileTargetSelected(actor, target int) {
	r := p.Flight
	if target == p.Initialized.PlayerRow && r.Systems.DamageMasks[1]&0x40 == 0 {
		r.SecondaryMFD = 5
	}
	p.limitFlightCompressionCause("hostile-target-selected", actor, target)
}
