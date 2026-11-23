package menus

// Only editor UI makes use host typematic; source03f8 consumes BIOS scan makes.
// Do not turn a held key into70Hz movement or share this state with recorded
// flight directions. Native repeat timing follows the host keyboard policy.
type editorArrowOwner struct {
	initialized   bool
	screen        Screen
	stage         string
	held, blocked [4]bool
}

func (c *FrontEnd) editorArrowMakes(a Actions, held, pressed, repeated [4]bool) Actions {
	if c.Editor == nil {
		return a
	}
	q := &c.Editor.ArrowOwner
	edges := [4]bool{a.Left, a.Right, a.Up, a.Down}
	active := c.Screen == ScreenMissionEditor && !a.FlightFocusLost && !a.PointerReset
	transition := q.initialized && (q.screen != c.Screen || q.stage != c.Editor.Stage)
	for i := 0; i < 4; i++ {
		if !held[i] {
			q.blocked[i] = false
		}
		if !active && held[i] || transition && held[i] && q.held[i] || !q.initialized && held[i] && !pressed[i] && !edges[i] {
			q.blocked[i] = true
		}
		if c.Screen == ScreenMissionEditor {
			edges[i] = active && !q.blocked[i] && (edges[i] || pressed[i] || repeated[i])
		}
	}
	q.initialized, q.screen, q.stage, q.held = true, c.Screen, c.Editor.Stage, held
	if c.Screen == ScreenMissionEditor {
		a.Left, a.Right, a.Up, a.Down = edges[0], edges[1], edges[2], edges[3]
	}
	return a
}
