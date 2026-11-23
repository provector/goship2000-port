package state

import "fmt"

// EnemyRules freezes the post-REALITY hostile definitions and policy before
// initialization. Nil is historical absent-A070 behavior, never an empty bank.
type EnemyRules struct {
	World       string
	Switches    uint16
	Disabled    bool
	Raw         [16][8]byte
	Definitions [16][8]byte
}

// FreezeEnemyRules is the original GS16F48 writer: byte additions wrap, with
// quality in SWITCHES low2 and the independent environment in its high byte.
// It consumes no random state and leaves the other six row bytes untouched.
func FreezeEnemyRules(world string, raw [16][8]byte, switches uint16, disabled bool) (*EnemyRules, error) {
	if world == "" {
		return nil, fmt.Errorf("enemy definitions need selected world identity")
	}
	r := &EnemyRules{World: world, Switches: switches, Disabled: disabled, Raw: raw, Definitions: raw}
	quality, environment := switches&3, (switches>>8)&3
	reload := [4]byte{72, 42, 18, 0}
	bypass := [4]byte{39, 25, 13, 0}
	for i := range r.Definitions {
		r.Definitions[i][2] += reload[quality] + byte(6*environment)
		r.Definitions[i][3] += bypass[quality]
	}
	return r, nil
}

func (r *EnemyRules) Validate(world string, switches uint16) error {
	if r == nil {
		return fmt.Errorf("stationary hostile profile needs frozen enemy definitions")
	}
	if r.World != world || r.Switches != switches {
		return fmt.Errorf("frozen enemy world/settings differ from accepted launch")
	}
	want, err := FreezeEnemyRules(world, r.Raw, switches, r.Disabled)
	if err != nil {
		return err
	}
	if want.Definitions != r.Definitions {
		return fmt.Errorf("frozen enemy definition transformation differs")
	}
	return nil
}
