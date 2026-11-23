package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
	"strings"
)

// These are view/input owners, never document/simulation/result RNG owners.
type EditorControls struct {
	Formation         bool
	Filter            uint16
	FilterInitialized bool
	ComponentPreview  int
	ControlChoice     int
	ControlReturn     string
	ListLimited       bool
}

func (c *FrontEnd) editorOptions(role string, kind, category int) ([]gamestate.AuthoredOption, error) {
	e := c.Editor
	cacheKey := fmt.Sprintf("%s/%d/%d/%t", role, kind, category, e.Formation)
	opts, ok := e.optionCache[cacheKey]
	limited := false
	var err error
	if ok {
		limited = e.optionLimited[cacheKey]
	} else {
		view := e.Document
		if view.Initial == nil {
			view, err = gamestate.NativeAuthoredEditCandidate(c.editorData(), view)
			if err != nil {
				return nil, err
			}
		}
		opts, limited, err = gamestate.AuthoredEditorOptions(c.editorData(), view, role, kind, category, e.Formation)
	}
	c.Editor.ListLimited = limited
	if len(opts) > 36 {
		opts = opts[:36]
	}
	c.Editor.ComponentPreview = 0
	key := "single_guidance"
	if c.Editor.Formation {
		key = "formation_guidance"
	}
	c.Editor.Status = c.editorAssets().Text[key]
	if limited {
		c.Editor.Status = c.editorAssets().Text["limited_prefix"] + " " + c.Editor.Status
	}
	if err == nil {
		if e.optionCache == nil {
			e.optionCache = make(map[string][]gamestate.AuthoredOption)
		}
		e.optionCache[cacheKey] = opts
		if e.optionLimited == nil {
			e.optionLimited = make(map[string]bool)
		}
		e.optionLimited[cacheKey] = limited
	}
	return opts, err
}

func (c *FrontEnd) editorPickedOptions(p gamestate.AuthoredPlacement) ([]gamestate.AuthoredOption, error) {
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), c.Editor.Document.World)
	if err != nil {
		return nil, err
	}
	if p.Component < 0 || p.Component*8+2 >= len(w.FRMD) {
		return nil, fmt.Errorf("picked source component is unavailable")
	}
	c.Editor.Formation = w.FRMD[p.Component*8+2] == 1
	c.Editor.Category = gamestate.AuthoredCategory(c.editorData(), c.Editor.Document, p)
	return c.editorOptions(p.Role, p.Kind, c.Editor.Category)
}

func (c *FrontEnd) placeEditorOption(d *gamestate.AuthoredDocument, role string, kind int, o gamestate.AuthoredOption, x, y int) error {
	return gamestate.PlaceNativeAuthoredOption(c.editorData(), d, role, kind, o, x, y)
}

func (c *FrontEnd) editorControlCommand(index int) bool {
	e := c.Editor
	switch index {
	case 12:
		e.Weapons = !e.Weapons
	case 13:
		e.Paths = !e.Paths
	case 14:
		if e.Stage == "filter" {
			e.Stage = "edit"
			return true
		}
		if !e.FilterInitialized {
			e.Filter = 0x3ff
			e.FilterInitialized = true
		}
		e.Stage = "filter"
		e.ControlChoice = 0
	case 15:
		e.All = !e.All
	case 17:
		// Source help is a toggle over the current selected component, not a new
		// generic instructions screen. Keep the actual selector context intact.
		if e.ControlReturn == "information" {
			e.ControlReturn = ""
		} else {
			e.ControlReturn = "information"
			if rows, err := c.editorPreviewRows(); err == nil && len(rows) > 0 {
				w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
				if err == nil {
					model := int(w.FRMD[rows[e.ComponentPreview%len(rows)]*8+1])
					if c.editorAssets().ModelInformation[model] == "" {
						e.Status = fmt.Sprintf("Source model %d information is opaque or unavailable.", model)
						c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
					}
				}
			}
		}
	case 18, 19:
		e.Formation = !e.Formation
		e.ComponentPreview = 0
		if e.Role == "defender" && (e.Held || e.Stage == "option") {
			e.Held = false
			if err := c.editorArm(e.Role, e.Kind, e.Category); err != nil {
				e.Status = err.Error()
				c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			}
		}
	default:
		return false
	}
	return true
}

func (c *FrontEnd) editorOpenControlMenu(kind string) error {
	e := c.Editor
	if e.supportCancellationGuarded() {
		e.Status = "The cancelled Support choice needs its prior editor state. Abort this draft to preserve the saved mission."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		return nil
	}
	if kind != "fill" && kind != "delete" {
		return fmt.Errorf("unknown editor control menu")
	}
	if e.Document.Initial != nil {
		e.Status = "Retained film geometry requires a verified inverse compiler."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		return nil
	}
	if kind == "fill" {
		if err := gamestate.ValidateAuthored(c.editorData(), e.Document); err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			return nil
		}
	}
	c.editorReleaseCommand()
	e.Stage = kind
	e.ControlChoice = 0
	e.Status = ""
	x := 88
	if kind == "delete" {
		x = 128
	}
	c.editorBeginPopup(x, 188, len(c.editorControlLabels()))
	e.ControlChoice = e.PopupHover
	return nil
}

func (c *FrontEnd) editorControlPopup() bool {
	s := c.Editor.Stage
	return s == "fill" || s == "delete" || s == "filter" || s == "control_delete_confirm"
}

func (c *FrontEnd) handleEditorControlPopup(a Actions, pointerActivity, select1, select2 bool) (bool, error) {
	e := c.Editor
	if !c.editorControlPopup() {
		return false, nil
	}
	if a.Cancel {
		if e.Stage == "fill" || e.Stage == "delete" {
			c.editorEndPopup()
		}
		e.Stage = "edit"
		e.Status = ""
		return true, nil
	}
	if e.Stage == "control_delete_confirm" {
		yes, no := a.ConfirmQuit, false
		for _, r := range a.Text {
			yes = yes || r == 'y' || r == 'Y'
			no = no || r == 'n' || r == 'N'
		}
		if a.PointerPressed && a.Inside {
			yes = yes || editorInclusive(a.X, a.Y, c.editorAssets().ExitRects["abort_yes"])
			no = no || editorInclusive(a.X, a.Y, c.editorAssets().ExitRects["abort_no"])
		}
		if no {
			e.Stage = "edit"
			return true, nil
		}
		if !yes {
			return true, nil
		}
		n, err := gamestate.AuthoredBulkDelete(c.editorData(), e.Document, e.ControlChoice)
		e.Stage = "edit"
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		} else {
			e.Dirty = e.Dirty || n > 0
			if n > 0 {
				e.optionCache = nil
			}
			e.Status = ""
		}
		return true, nil
	}
	if e.Stage == "filter" {
		//20f4:5887..58BE exits on the same AREA29F4/scan3D that
		// entered ICON. Category selectors toggle a bit; ICON itself does
		// not toggle whichever category happened to remain under the cursor.
		hitIcon := false
		if a.PointerPressed && a.Inside {
			for _, r := range c.editorAssets().Rectangles {
				if r.Group == "category_and_view" && r.Index == 14 && r.Contains(a.X, a.Y) {
					hitIcon = true
					break
				}
			}
		}
		if a.Enter || a.ViewKey == 3 || hitIcon {
			e.Stage = "edit"
			return true, nil
		}
		if pointerActivity && a.Inside {
			for _, r := range c.editorAssets().Rectangles {
				if r.Group == "category_and_view" && r.Index >= 1 && r.Index <= 10 && r.Contains(a.X, a.Y) {
					e.ControlChoice = r.Index - 1
					break
				}
			}
		}
		if a.Up || a.Left {
			e.ControlChoice = (e.ControlChoice + 9) % 10
		}
		if a.Down || a.Right {
			e.ControlChoice = (e.ControlChoice + 1) % 10
		}
		if select1 || select2 || a.Skip {
			e.Filter ^= 1 << e.ControlChoice
		}
		return true, nil
	}
	labels := c.editorControlLabels()
	count := len(labels)
	if count == 0 {
		e.Status = "Prepared source editor menu labels are unavailable."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		e.Stage = "edit"
		return true, nil
	}
	c.editorMoveCursor(a, pointerActivity)
	c.editorPopupHover(count)
	e.ControlChoice = e.PopupHover

	if select1 && e.PopupHover >= 0 {
		c.editorEndPopup()
		if e.Stage == "delete" {
			e.Stage = "control_delete_confirm"
			return true, nil
		}
		n, err := gamestate.FillNativeAuthored(c.editorData(), e.Document, e.ControlChoice)
		e.Stage = "edit"
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		} else {
			e.Dirty = e.Dirty || n > 0
			if n > 0 {
				e.optionCache = nil
			}
			e.Status = ""
		}
	}
	return true, nil
}

func (c *FrontEnd) editorControlLabels() []string {
	if c.Editor.Stage == "fill" {
		return c.editorAssets().FillLabels
	}
	return c.editorAssets().DeleteLabels
}

func (c *FrontEnd) editorCategoryVisible(index int) bool {
	e := c.Editor
	if index < 0 || index >= 10 || e.All || !e.FilterInitialized {
		return true
	}
	return e.Filter&(1<<index) != 0
}

func (c *FrontEnd) editorPreviewRows() ([]int, error) {
	e := c.Editor
	if e.Option < 0 || e.Option >= len(e.Options) {
		return nil, nil
	}
	o := e.Options[e.Option]
	p := gamestate.AuthoredPlacement{Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile}
	return gamestate.AuthoredComponents(c.editorData(), e.Document, p)
}

func (c *FrontEnd) editorCyclePreview() {
	rows, err := c.editorPreviewRows()
	if err == nil && len(rows) > 0 {
		c.Editor.ComponentPreview = (c.Editor.ComponentPreview + 1) % len(rows)
	}
}

func (c *FrontEnd) drawEditorMapOverlays(frame []byte) {
	e := c.Editor
	if e == nil || e.Document == nil {
		return
	}
	if e.Weapons || e.All {
		c.drawEditorEffectiveness(frame)
	}
	if e.Paths || e.All {
		c.drawEditorPaths(frame)
	}
}

func (c *FrontEnd) drawEditorControlOverlay(frame []byte) {
	e := c.Editor
	if e == nil || e.Document == nil || len(frame) != 64000 {
		return
	}
	for _, r := range c.editorAssets().Rectangles {
		if r.Group != "category_and_view" || r.Index < 1 || r.Index == 11 || r.Index > 19 {
			continue
		}
		on := r.Index >= 1 && r.Index <= 10 && e.Role == "defender" && e.Category == r.Index
		switch r.Index {
		case 12:
			on = e.Weapons
		case 13:
			on = e.Paths
		case 14:
			on = e.Stage == "filter"
		case 15:
			on = e.All
		case 16:
			on = e.View.Detail
		case 17:
			on = e.ControlReturn == "information"
		case 18:
			// AREA18 is the upper single-model icon; sourceC01A==0.
			on = !e.Formation
		case 19:
			on = e.Formation
		}
		// The image bank contains baked edges. Painting only the selected state
		// leaves the formerly selected button depressed after a toggle.
		drawEditorButtonBevel(frame, [4]int{r.X, r.Y, r.Width, r.Height}, on)
	}

	if e.Stage == "filter" {
		for _, r := range c.editorAssets().Rectangles {
			if r.Group != "category_and_view" || r.Index < 1 || r.Index > 10 {
				continue
			}
			if e.Filter&(1<<(r.Index-1)) != 0 {
				drawEditorBevel(frame, [4]int{r.X, r.Y, r.Width, r.Height})
			}
			if r.Index-1 == e.ControlChoice {
				_ = assetdata.OfficeLine(frame, r.X, r.Y, r.X+r.Width, r.Y, 0x44)
			}
		}
	}
	if e.Stage == "option" && e.PopupHover >= 0 {
		c.drawEditorComponentPreview(frame)
	}
	if e.Stage == "fill" || e.Stage == "delete" {
		c.drawEditorPopup(frame, c.editorControlLabels())
	}

	if e.Stage == "control_delete_confirm" {
		c.drawEditorDeleteConfirm(frame)
	}
}

// Source20f4:3b02..3eee previews the actual FRMD arrangement with the source
// icon atlas. Selector2 changes only the highlighted component and information.
func (c *FrontEnd) drawEditorComponentPreview(frame []byte) {
	e, a := c.Editor, c.editorAssets()
	rows, err := c.editorPreviewRows()
	if err != nil || len(rows) == 0 {
		return
	}
	w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	if err != nil {
		return
	}
	assetdata.OfficeFill(frame, 169, 139, 149, 59, 3)
	assetdata.OfficeFill(frame, 170, 140, 147, 57, 9)
	assetdata.OfficeFill(frame, 171, 141, 145, 55, 1)
	o := e.Options[e.Option]
	if o.Formation >= 0 && o.Formation < len(w.FMHD)/39 && binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])&0x800000 != 0 {
		c.clippedText(frame, 3, c.editorAssets().Text["special_formation"], 215, 166, 15, [4]int{171, 141, 145, 55})
		return
	}
	first := rows[0]
	model := int(w.FRMD[first*8+1])
	cat := 0
	types := c.editorData().Tables.EditorObjectTypes
	if model*27+1 < len(types) {
		cat = int(types[model*27+1])
	}
	xscale, yscale, previewScale := 30, 50, 0
	if model == 0xcd || model == 0x93 || cat == 8 {
		xscale, yscale, previewScale = 60, 100, 1
	}
	if cat == 5 {
		xscale, yscale, previewScale = 120, 200, 2
	}
	if model == 0x40 {
		xscale, yscale, previewScale = 480, 800, 4
	}
	lo, hi := 0, 0
	for _, ci := range rows {
		y := int(int16(binary.LittleEndian.Uint16(w.FRMD[ci*8+6:])))
		lo = min(lo, y)
		hi = max(hi, y)
	}
	mid := (lo + hi) / 2
	selected := e.ComponentPreview % len(rows)
	for i, ci := range rows {
		b := w.FRMD[ci*8:]
		model := int(b[1])
		if model*27+1 >= len(types) {
			continue
		}
		cat := min(int(types[model*27+1]), 9)
		if cat < 0 || cat >= len(a.IconRects) {
			continue
		}
		r := a.IconRects[cat]
		if cat > 8 && b[3] == 0 {
			r[1] += 4
		}
		if i == selected {
			r[1] += 16
		}
		x, y := 277, 166
		if len(rows) > 1 {
			x += int(int16(binary.LittleEndian.Uint16(b[4:]))) / xscale
			y += (mid - int(int16(binary.LittleEndian.Uint16(b[6:])))) / yscale
		}
		for sy := 0; sy < r[3]; sy++ {
			for sx := 0; sx < r[2]; sx++ {
				dx, dy := x+sx, y+sy
				at := (r[1]+sy)*320 + r[0] + sx
				if dx < 171 || dx > 315 || dy < 141 || dy > 195 || at < 0 || at >= len(a.Icons) {
					continue
				}
				if v := a.Icons[at]; v != 0 {
					frame[dy*320+dx] = v
				}
			}
		}
	}
	// Source captions use the same text styles/row origins as3f87..4099.
	name := fmt.Sprintf("[%d] %s", len(rows), o.Name)
	c.clippedText(frame, 3, name, 174, 143, 10, [4]int{174, 143, 140, 6})
	if previewScale > 0 {
		c.clippedText(frame, 3, fmt.Sprintf("[%d:1]%s", previewScale+1, c.editorText("magnification", " Magnification")), 174, 149, 10, [4]int{174, 149, 140, 6})
	}
	ci := rows[selected]
	model = int(w.FRMD[ci*8+1])
	if model*27+27 <= len(types) {
		name = editorCString(types[model*27+2:][:25])
		c.clippedText(frame, 3, fmt.Sprintf("%s%d:  %s", c.editorText("object_number", "Object #"), selected+1, name), 174, 176, 10, [4]int{174, 176, 140, 6})
	}
	prefix := strings.TrimRight(e.Document.World, "0123456789")
	wp := a.WeaponTypes[prefix]
	weapon := int(w.FRMD[ci*8+3])
	primaryWeapon, secondaryWeapon := weapon>>4, weapon&15
	if len(wp) == 16*27 {
		label := c.editorText("weapons_prefix", "Weapons: ")
		c.clippedText(frame, 3, label+editorCString(wp[primaryWeapon*27+1:][:26]), 174, 183, 10, [4]int{174, 183, 140, 6})
		if primaryWeapon != secondaryWeapon {
			c.clippedText(frame, 3, label+editorCString(wp[secondaryWeapon*27+1:][:26]), 174, 189, 10, [4]int{174, 189, 140, 6})
		}
	}

	if e.ControlReturn == "information" {
		// Use only the source supplied description for this exact model. A
		// missing description never substitutes an invented generic fact sheet.
		if text := a.ModelInformation[model]; text != "" {
			area := [4]int{171, 11, 145, 90}
			for _, r := range a.Rectangles {
				if r.Group == "parameter" && r.Index == 0 {
					area = [4]int{r.X + 2, r.Y + 2, r.Width - 4, 88}
					break
				}
			}
			assetdata.OfficeFill(frame, area[0], area[1], area[2], area[3], 0x58)
			c.editorInformationText(frame, text, area)
		}
	}
}

func editorCString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func (c *FrontEnd) drawEditorPaths(frame []byte) {
	d := c.Editor.Document
	lines := gamestate.AuthoredDisplayPaths(d)
	if d.Initial == nil {
		if c.Editor.routeOverlayCache == nil {
			c.Editor.routeOverlayCache = make(map[editorRouteKey][][2]int)
		}
		for _, p := range d.Placements {
			if p.Route == nil {
				continue
			}
			key := editorRouteKey{d.World, p.ID, p.Formation, p.Component, p.X, p.Y, p.Route.X, p.Route.Y, p.Route.Profile}
			line, ok := c.Editor.routeOverlayCache[key]
			if !ok {
				var err error
				line, err = gamestate.AuthoredRouteDisplay(c.editorData(), d, p)
				if err != nil {
					continue
				}
				if len(c.Editor.routeOverlayCache) >= 4 {
					clear(c.Editor.routeOverlayCache)
				}
				c.Editor.routeOverlayCache[key] = line
			}
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		for i := 1; i < len(line); i++ {
			x0, y0 := c.editorScreen(line[i-1][0], line[i-1][1])
			x1, y1 := c.editorScreen(line[i][0], line[i][1])
			c.editorClippedControlLine(frame, x0, y0, x1, y1, 3)
		}
	}
}

func (c *FrontEnd) editorClippedControlLine(frame []byte, x0, y0, x1, y1 int, ink byte) {
	// A validated source endpoint is within0..7ffff; conversion to the1536x1152
	// editor map and a clamped camera keep projected coordinates inside8192.
	// Reject malformed presentation inputs without clamping their endpoints,
	// which would change the source line's error phase at the viewport edge.
	if len(frame) != 64000 || x0 < -8192 || x0 > 8192 || x1 < -8192 || x1 > 8192 || y0 < -8192 || y0 > 8192 || y1 < -8192 || y1 > 8192 {
		return
	}
	b := c.editorMapBounds()
	// This is OfficeLine's exact inclusive stepping/tie rule, with masked writes
	// instead of its endpoint admission check. Retain the full original segment
	// so clipping does not restart the raster at a rounded intersection.
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}
	dx, dy := x1-x0, y1-y0
	vertical := 1
	if dy < 0 {
		vertical = -1
		dy = -dy
	}
	major, minor, ax, ay, bx, by := dx, dy, 1, 0, 0, vertical
	if dx < dy {
		major, minor, ax, ay, bx, by = dy, dx, bx, by, ax, ay
	}
	errorTerm := -((major + 1) / 2)
	x, y := x0, y0
	for i := 0; i <= major; i++ {
		if x >= max(0, b[0]) && x <= min(319, b[0]+b[2]) && y >= max(0, b[1]) && y <= min(199, b[1]+b[3]) {
			frame[y*320+x] = ink
		}
		if i == major {
			break
		}
		x += ax
		y += ay
		errorTerm += minor
		if errorTerm >= 0 {
			errorTerm -= major
			x += bx
			y += by
		}
	}
}

// Source20f4:54a7..55f0 takes both weapon nibbles, indexes 27-byte
// records and passes byte0 directly as a pixel radius with ink9f to08ee.
func (c *FrontEnd) drawEditorEffectiveness(frame []byte) {
	e := c.Editor
	w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	if err != nil {
		return
	}
	prefixes := [...]string{"EURO", "GULF", "ANTC", "PHIL"}
	if w.Theatre < 0 || w.Theatre >= len(prefixes) {
		return
	}
	weapons := c.editorAssets().WeaponTypes[prefixes[w.Theatre]]
	types := c.editorData().Tables.EditorObjectTypes
	if len(weapons) < 16*27 {
		return
	}
	for _, p := range e.Document.Placements {
		cat := gamestate.AuthoredCategory(c.editorData(), e.Document, p) - 1
		if !c.editorCategoryVisible(cat) || cat == 5 && w.FRMD[p.Component*8+2] == 2 {
			continue
		}
		rows, err := gamestate.AuthoredComponents(c.editorData(), e.Document, p)
		if err != nil {
			continue
		}
		x, y := c.editorScreen(p.X, p.Y)
		for _, ci := range rows {
			b := w.FRMD[ci*8:]
			model := int(b[1])
			if model*27+1 >= len(types) {
				continue
			}
			for _, wi := range []byte{b[3] & 15, b[3] >> 4} {
				radius := int(weapons[int(wi)*27])
				if radius > 0 {
					c.editorControlCircle(frame, x, y, radius, 0x9f)
				}
			}
		}
	}
}

func (c *FrontEnd) editorControlCircle(frame []byte, cx, cy, r int, ink byte) {
	if r < 0 || r > 255 {
		return
	}
	b := c.editorMapBounds()
	put := func(x, y int) {
		if x >= b[0] && x <= b[0]+b[2] && y >= b[1] && y <= b[1]+b[3] && x >= 0 && x < 320 && y >= 0 && y < 200 {
			frame[y*320+x] = ink
		}
	}
	plot := func(x, y int) {
		for _, p := range [][2]int{{x, y}, {-x, y}, {x, -y}, {-x, -y}, {y, x}, {-y, x}, {y, -x}, {-y, -x}} {
			put(cx+p[0], cy+p[1])
		}
	}
	x, y, decision, inc, dec := 0, r, 1-r, 3, 5-2*r
	plot(x, y)
	for x < y {
		x++
		inc += 2
		dec += 2
		if decision < 0 {
			decision += inc
		} else {
			decision += dec
			dec += 2
			y--
		}
		plot(x, y)
	}
}

func (c *FrontEnd) editorInformationText(frame []byte, text string, area [4]int) {
	font := c.Menus.Fonts[3]
	if font == nil {
		return
	}
	x, y := area[0]+2, area[1]+2
	line := ""
	flush := func() { c.clippedText(frame, 3, line, x, y, 11, area); y += 7; line = "" }
	for _, r := range text {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			flush()
			continue
		}
		if font.Measure([]byte(line+string(r))) > area[2]-4 {
			flush()
		}
		line += string(r)
		if y >= area[1]+area[3]-6 {
			break
		}
	}
	if line != "" {
		flush()
	}
}

// At most two current source objectives and a bounded old-edit cache. Neither
// drawing nor hover recompiles a document or consumes its stored CRT stream.
type editorRouteKey struct {
	World, ID                                              string
	Formation, Component, X, Y, DestinationX, DestinationY int
	Profile                                                string
}
