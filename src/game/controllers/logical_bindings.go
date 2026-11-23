package controllers

import "reflect"

// Keep Q2's stored IDs readable. Consolidation is an explicit draft edit, never
// a load-time migration or an unrelated-save side effect.
func LogicalID(id string) string {
	switch id {
	case "weapon.fire":
		return "menu.selector1"
	case "weapon.target":
		return "menu.selector2"
	}
	return id
}
func BindingIDs(id string) []string {
	switch LogicalID(id) {
	case "menu.selector1":
		return []string{"menu.selector1", "weapon.fire"}
	case "menu.selector2":
		return []string{"menu.selector2", "weapon.target"}
	}
	return []string{id}
}
func EffectiveBinding(p Profile, id string) (Binding, bool) {
	if d, ok := DirectionByID(id); ok {
		pair := ButtonPair(p, d.Axis)
		c := pair.Negative
		positive := d.Positive
		if pair.Invert {
			positive = !positive
		}
		if positive {
			c = pair.Positive
		}
		if c == nil {
			return Binding{}, false
		}
		return Binding{Kind: "control", Control: c}, false
	}
	var found Binding
	have := false
	for _, key := range BindingIDs(id) {
		b := p.Bindings[key]
		if b.Kind == "" && !b.Invert {
			continue
		}
		if have && !reflect.DeepEqual(found, b) {
			return Binding{}, true
		}
		found = b
		have = true
	}
	return found, false
}
func SetBinding(p *Profile, id string, b Binding) {
	if d, ok := DirectionByID(id); ok {
		setDirection(p, d, b)
		return
	}
	if a, ok := ActionByID(id); ok && a.Axis {
		old := p.Bindings[id]
		if b.Kind == "axis" && old.Kind == "pair" {
			if p.AxisButtons == nil {
				p.AxisButtons = map[string]Binding{}
			}
			pair := old
			if pair.Invert {
				pair.Negative, pair.Positive = pair.Positive, pair.Negative
			}
			pair.Invert = false
			p.AxisButtons[id] = pair
		}
		if b.Kind == "pair" {
			delete(p.AxisButtons, id)
		}
	}
	for _, key := range BindingIDs(id) {
		delete(p.Bindings, key)
	}
	if b.Kind != "" || b.Invert {
		p.Bindings[LogicalID(id)] = b
	}
}

// One editable row per logical selector. Preserve all inventory references,
// contexts and keyboard aliases in the consolidated rows and tooltips.
func MappingActions() []Action {
	var out []Action
	seen := map[string]bool{}
	for _, a := range Actions {
		if LogicalID(a.ID) != a.ID || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		switch a.ID {
		case "menu.selector1":
			a.Label = "Selector #1 (Fire in flight)"
			a.Keyboard = "Enter (menu); Enter/KPEnter (flight)"
			a.Contexts = []string{"menus", "editor", "flight"}
			a.Reference += "; weapon.fire: Enter/KPEnter in flight (same logical controller); fixed keyboard remains context-specific"
		case "menu.selector2":
			a.Label = "Selector #2 (Change Target in flight)"
			a.Keyboard = "Space (menu/editor); Backspace (flight)"
			a.Contexts = []string{"menus", "editor", "flight"}
			a.Reference += "; weapon.target: Backspace in flight; Editor Tools Single/Group alias: Space; fixed keyboard remains context-specific"
		case "weapon.weapon":
			a.Label = "Switch weapon"
		}
		out = append(out, a)
	}
	return out
}
func MappingAction(id string) (Action, bool) {
	if d, ok := DirectionByID(id); ok {
		return directionAction(d), true
	}
	id = LogicalID(id)
	for _, a := range MappingActions() {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}
