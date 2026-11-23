package menus

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// Both the FARP control and picking the actual FARP marker reach source205C.
// The F control's11CA preparation checks are performed by its caller first.
func (c *FrontEnd) editorLiftFARP() (bool, error) {
	e := c.Editor
	var existing *gamestate.AuthoredPlacement
	for _, p := range e.Document.Placements {
		if p.Role == "FARP" {
			q := p
			existing = &q
			break
		}
	}
	if existing == nil {
		return false, nil
	}
	w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	if err != nil {
		return false, err
	}
	options, err := gamestate.AuthoredOptions(c.editorData(), e.Document, "FARP", 0)
	if err != nil {
		return false, err
	}
	var option *gamestate.AuthoredOption
	for _, o := range options {
		if o.Formation == existing.Formation && o.Component == existing.Component && o.Template == existing.Template {
			q := o
			option = &q
			break
		}
	}
	if option == nil {
		return false, fmt.Errorf("placed FARP lacks its source construction option")
	}
	lifted, err := gamestate.LiftAuthoredFARP(c.editorData(), e.Document)
	if err != nil {
		return false, err
	}
	c.editorReleaseCommand()
	e.LiftedFARP = lifted
	e.Role, e.Kind, e.Category = "FARP", 0, 0
	e.Options = []gamestate.AuthoredOption{*option}
	e.Option, e.Stage, e.Held = 0, "edit", true
	e.Dirty = true
	e.optionCache = nil
	//The held descriptor owns its exact source component, not an arbitrary new
	//choice. Fixed-site semantics still come from the validated FMHD flags.
	e.Options[0].Fixed = binary.LittleEndian.Uint32(w.FMHD[lifted.Formation*39+35:])&0x1000 != 0
	c.updateEditorShadow()
	return true, nil
}
