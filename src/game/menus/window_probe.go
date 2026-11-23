package menus

// windowProbe describes UI transport only. It is used by explicit development
// captures to distinguish real OS input from controller-only assertions.
func (c *FrontEnd) windowProbe() map[string]any {
	out := map[string]any{"screen": c.Screen, "previous_screen": c.previousScreen, "message": c.Message}
	if l := c.Library; l != nil {
		out["library"] = map[string]any{"count": len(l.Entries), "selected": l.Selected, "page": l.Page, "editing": l.Editing, "status": l.Status}
	}
	if f := c.Files; f != nil {
		out["mission_files"] = map[string]any{"count": len(f.Entries), "selected": f.Selected, "status": f.Status, "editing": f.Editing}
	}
	if e := c.Editor; e != nil {
		id := ""
		count := 0
		if e.Document != nil {
			id = e.Document.ID
			count = len(e.Document.Placements)
		}
		out["editor"] = map[string]any{"stage": e.Stage, "status": e.Status, "held": e.Held, "role": e.Role, "kind": e.Kind, "option": e.Option, "shadow_error": e.ShadowError, "document_id": id, "placements": count}
	}
	if v := c.Viewer; v != nil {
		out["viewer"] = map[string]any{"stopped": v.Stopped, "practice": v.Practice, "fast": v.Fast, "next": v.Next, "map": v.Map, "demo": v.Demo, "actor": v.Actor, "cursor": v.Playback.Cursor, "divergence": v.Playback.Divergence, "child_screen": v.Playback.Engine.Screen}
	}
	var preview *RenderingPreview
	if c.Viewer != nil && c.Viewer.Playback != nil {
		preview = c.Viewer.Playback.Preview()
	} else {
		preview = c.Rendering
	}
	if preview != nil && preview.Flight != nil {
		f := preview.Flight
		out["flight"] = map[string]any{"rotor": f.State.Rotor, "collective": f.State.Collective, "pose": f.State.Pose, "steps": f.Steps}
	}
	return out
}
