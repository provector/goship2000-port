package menus

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"sort"
	"strings"
)

type ordnanceMenu struct {
	Kind     string
	System   int
	Selected int
}

type OrdnanceChoice struct {
	Text, Action  string
	Value, System int
	Enabled       bool
	Rectangle     [4]int
	Station       int
}

// OrdnanceView edits one source-shaped configuration. Only normal ACCEPT copies
// it back, except aircraft replacement, whose original handler resets the live
// selected row immediately. No operation here generates or executes a mission.
type OrdnanceView struct {
	Assets     *assetdata.MissionAssets
	Mission    *gamestate.MissionData
	Flight     []gamestate.FlightPilot
	Rows       [][36]byte
	Slot       int
	Rank       gamestate.Rank
	Working    [36]byte
	Choices    []OrdnanceChoice
	Done       bool
	Message    string
	Hover      int
	audioCues  []uint16 // Per-action source menu dispatch and optional rejection.
	stack      []ordnanceMenu
	rowPointer ordnanceRowPointer
}

func NewOrdnanceView(a *assetdata.MissionAssets, m *gamestate.MissionData, flight []gamestate.FlightPilot, rows [][36]byte, slot int, rank gamestate.Rank) (*OrdnanceView, error) {
	if a == nil || m == nil || a.Preflight == nil || slot < 0 || slot >= len(rows) || slot >= len(flight) || rank.Raw() < 0 {
		return nil, fmt.Errorf("ordnance requires a prepared aircraft/pilot slot")
	}
	v := &OrdnanceView{Assets: a, Mission: m, Flight: flight, Rows: rows, Slot: slot, Rank: rank, Working: rows[slot], Hover: -1, stack: []ordnanceMenu{{Kind: "main"}}}
	if _, ok := a.Preflight.Ships[int(int8(v.Working[0]))]; !ok {
		return nil, fmt.Errorf("aircraft%d presentation unavailable", v.Working[0])
	}
	v.rebuild()
	return v, nil
}

func (v *OrdnanceView) ship() assetdata.ShipPresentation {
	return v.Assets.Preflight.Ships[int(int8(v.Working[0]))]
}

func (v *OrdnanceView) definition() []byte {
	return v.Assets.AircraftDefinitions[int(int8(v.Working[0]))]
}

func preflightCString(b []byte, at int) string {
	if at < 0 || at >= len(b) {
		return ""
	}
	return assetdata.OfficeCString(b[at:])
}

func (v *OrdnanceView) hasSystem(id int) bool {
	for _, f := range v.ship().Fits {
		if f.System == id {
			return true
		}
	}
	return false
}

func (v *OrdnanceView) systemIndex(id int) int {
	for i, s := range v.Assets.Preflight.SystemOrder {
		if s == id {
			return i
		}
	}
	return -1
}

func preflightDenied(values []int, id int) bool {
	for _, n := range values {
		if n == id {
			return true
		}
	}
	return false
}

func (v *OrdnanceView) text(ds int, fallback string) string {
	if s := v.Assets.Preflight.Text[ds]; s != "" {
		return s
	}
	return fallback
}

func (v *OrdnanceView) add(text, action string, value, system, station int, enabled bool) {
	v.Choices = append(v.Choices, OrdnanceChoice{Text: text, Action: action, Value: value, System: system, Station: station, Enabled: enabled})
}

func (v *OrdnanceView) rebuild() {
	v.Choices = nil
	menu := v.stack[len(v.stack)-1]
	r, sh := v.Working[:], v.definition()
	switch menu.Kind {
	case "main":
		v.add(v.text(0x401d, "WEAPON SYSTEMS"), "heading", 0, 0, -1, false)
		ids := make([]int, 0, len(v.Assets.WeaponDefinitions))
		for id := range v.Assets.WeaponDefinitions {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			d := v.Assets.WeaponDefinitions[id]
			if !v.hasSystem(id) || d[1] == 0 || preflightDenied(v.Mission.CampaignUnavailableSystems, v.systemIndex(id)) {
				continue
			}
			v.add(preflightCString(d, 26), "fits", 0, id, -1, true)
		}
		// S0ffd calls A25c32 (requires byte1!=0), then S100d requires
		// byte1==0. The apparent SHIP STUFF heading has no reachable rows.
		v.add(v.text(0x4037, "OPTIONS"), "heading", 0, 0, -1, false)
		for station := 0; station < 3; station++ {
			if gamestate.RowWord(r, 12+2*station) != 0 {
				v.add(v.text(0x403f, "REMOVE WEAPONS"), "remove_menu", 0, 0, -1, true)
				break
			}
		}
		v.add(v.text(0x404e, "CHANGE HELICOPTER"), "aircraft", 0, 0, -1, true)
		for _, id := range ids {
			d := v.Assets.WeaponDefinitions[id]
			if !v.hasSystem(id) || d[1] != 0 {
				continue
			}
			flags := int(gamestate.RowWord(d, 4))
			if flags&0x4000 != 0 {
				for i, f := range v.ship().Fits {
					if f.System == id {
						// Port convenience: cargo and squad compete for one station;
						// reselecting uses S12ba removal. Native S11ec only remounts.
						if int(int8(r[3+f.Station])) == id && gamestate.RowWord(r, 12+2*f.Station) > 0 {
							v.add(preflightCString(d, 26), "remove", f.Station, id, f.Station, true)
						} else {
							v.add(preflightCString(d, 26), "mount", i, id, f.Station, true)
						}
						break
					}
				}
			} else {
				v.add(preflightCString(d, 26), "quantity_menu", 0, id, -1, true)
			}
		}
	case "fits":
		v.add(v.text(0x3f60, "MOUNT:"), "heading", 0, 0, -1, false)
		d := v.Assets.WeaponDefinitions[menu.System]
		for i, f := range v.ship().Fits {
			if f.System != menu.System || f.Station > 2 {
				continue
			}
			q := int(int16(int(int8(sh[9])) * f.Quantity))
			if int(int8(r[3+f.Station])) == f.System && int(gamestate.RowWord(r, 12+2*f.Station)) == q {
				continue
			}
			label := fmt.Sprintf(v.text(0x3f67, "%dx %s ON\n  "), q, preflightCString(d, 17)) + preflightCString(sh, 0x50+17*f.Station)
			v.add(label, "mount", i, menu.System, f.Station, true)
		}
		for station := 0; station < 3; station++ {
			if int(int8(r[3+station])) == menu.System && gamestate.RowWord(r, 12+2*station) > 0 {
				label := fmt.Sprintf(v.text(0x3f7c, "%dx %s FROM\n  "), gamestate.RowWord(r, 12+2*station), preflightCString(d, 17)) + preflightCString(sh, 0x50+17*station)
				v.add(label, "remove", station, menu.System, station, true)
			}
		}
	case "remove":
		v.add(v.text(0x3f3e, "REMOVE:"), "heading", 0, 0, -1, false)
		for station := 0; station < 3; station++ {
			if gamestate.RowWord(r, 12+2*station) > 0 {
				d := v.Assets.WeaponDefinitions[int(int8(r[3+station]))]
				v.add(fmt.Sprintf(v.text(0x3f46, "%dx %s FROM\n  "), gamestate.RowWord(r, 12+2*station), preflightCString(d, 17))+preflightCString(sh, 0x50+17*station), "remove", station, 0, station, true)
			}
		}
		v.add(v.text(0x3f55, "REMOVE ALL"), "remove_all", 0, 0, -1, true)
	case "aircraft":
		v.add(v.text(0x3ea9, "SELECTIONS:"), "heading", 0, 0, -1, false)
		for id := 0; id < 8; id++ {
			d := v.Assets.AircraftDefinitions[id]
			if preflightDenied(v.Mission.CampaignUnavailableAircraft, id) {
				continue
			}
			v.add(preflightCString(d, 38), "replace", id, 0, -1, int(int8(d[5])) <= v.Rank.Raw())
		}
	case "quantity":
		d := v.Assets.WeaponDefinitions[menu.System]
		at, step, limit := ordnanceQuantity(r, sh, d)
		current := int(gamestate.RowWord(r, at))
		detail := fmt.Sprintf("Pounds: %d\nLimit: %d", current, limit)
		if int(gamestate.RowWord(d, 4))&2 != 0 {
			detail = fmt.Sprintf("Rounds: %d\nLimit: %d", current, limit)
		}
		if int(gamestate.RowWord(d, 4))&0x100 != 0 {
			detail = fmt.Sprintf("Chaff: %d\nFlares: %d", current, 60-current)
		}
		v.add(detail, "heading", 0, 0, -1, false)
		up, down := int(int16(current+step)), int(int16(current-step))
		if up > limit {
			up = limit
		}
		if down < 0 {
			down = 0
		}
		values := [4]int{limit, up, down, 0}
		labels := [4]string{"FILL", "INCREASE", "DECREASE", "EMPTY"}
		ds := [4]int{0x4000, 0x4005, 0x400e, 0x4017}
		if int(gamestate.RowWord(d, 4))&0x100 != 0 {
			labels = [4]string{"FULL CHAFF", "INCREASE CHAFF", "INCREASE FLARES", "FULL FLARES"}
			ds = [4]int{0x3fa8, 0x3fb3, 0x3fc2, 0x3fd2}
		}
		for i, value := range values {
			v.add(v.text(ds[i], labels[i]), "quantity", value, menu.System, -1, true)
		}
	}
	label := "ORDNANCE MENU"
	if len(v.stack) == 1 {
		label = "ACCEPT"
	}
	v.add(label, "back", 0, 0, -1, true)
	// The original right-hand menu is data-driven and wraps long rows. Keep the
	// same tiny bitmap font and clipping; no whole-screen canvas scaling here.
	y := 67 // S14e4 reserves the top 67 rows for the selected item's preview.
	for i := range v.Choices {
		if v.Choices[i].Action == "back" {
			v.Choices[i].Rectangle = [4]int{236, 178, 78, 10}
			continue
		}
		lines := ordnanceWrap(v.Assets.Fonts[3], v.Choices[i].Text, 76)
		h := 6 * len(lines)
		if h < 6 {
			h = 6
		}
		if v.Choices[i].Action == "heading" {
			h += 2
		}
		v.Choices[i].Rectangle = [4]int{229, y, 91, h}
		y += h
	}
	v.restoreRowSelection()
}

func ordnanceQuantity(r, sh, d []byte) (at, step, limit int) {
	flags := int(gamestate.RowWord(d, 4))
	if flags&0x100 != 0 {
		return 31, 3, 60
	}
	if flags&2 != 0 {
		return 21, 100, int(gamestate.RowWord(d, 2))
	}
	return 29, 50 * int(int8(sh[4])), int(gamestate.RowWord(r, 27))
}

func (v *OrdnanceView) Hit(x, y int) int {
	for i := len(v.Choices) - 1; i >= 0; i-- {
		if v.Choices[i].Action != "heading" && insideRect(x, y, v.Choices[i].Rectangle) && y < 191 {
			return i
		}
	}
	return -1
}

func (v *OrdnanceView) Handle(a Actions) (bool, error) {
	v.updateRowPointer(a)
	if a.Selector1 && (a.Inside || !a.PointerPressed) {
		if v.Hover >= 0 {
			return v.Activate(v.Hover)
		}
		v.audioCues = append(v.audioCues, gs4Denied) // S14c6 nonpositive menu hit.
	}
	return v.Done, nil
}

func (v *OrdnanceView) Activate(index int) (bool, error) {
	if index < 0 || index >= len(v.Choices) || v.Done {
		return v.Done, nil
	}
	c := v.Choices[index]
	v.Message = ""
	if c.Action == "heading" {
		return false, nil
	}
	v.stack[len(v.stack)-1].Selected = index
	v.audioCues = append(v.audioCues, gs4Accepted) // Positive-row dispatcherS1468.
	if !c.Enabled {
		v.audioCues = append(v.audioCues, gs4Denied) // Unavailable callbackS13ad.
		return false, nil
	}
	// A callback may add21h after the positive dispatch22h, including top
	// ACCEPT rejection. Those are two actual ordered source calls.
	r, sh := v.Working[:], v.definition()
	switch c.Action {
	case "fits":
		v.stack = append(v.stack, ordnanceMenu{Kind: "fits", System: c.System})
	case "remove_menu":
		v.stack = append(v.stack, ordnanceMenu{Kind: "remove"})
	case "aircraft":
		v.stack = append(v.stack, ordnanceMenu{Kind: "aircraft"})
	case "quantity_menu":
		v.stack = append(v.stack, ordnanceMenu{Kind: "quantity", System: c.System})
	case "quantity":
		at, _, _ := ordnanceQuantity(r, sh, v.Assets.WeaponDefinitions[c.System])
		gamestate.RowPut(r, at, c.Value)
		gamestate.RowPut(r, 33, 60-int(gamestate.RowWord(r, 31)))
	case "mount":
		f := v.ship().Fits[c.Value]
		if f.Station < 0 || f.Station > 2 {
			return false, fmt.Errorf("source option is not a mountable station")
		}
		r[3+f.Station], r[18+f.Station] = byte(f.System), byte(c.Value)
		gamestate.RowPut(r, 12+2*f.Station, int(int8(sh[9]))*f.Quantity)
		flags := int(gamestate.RowWord(v.Assets.WeaponDefinitions[f.System], 4))
		if flags&0x4000 != 0 {
			r[35] = 2
			if flags&0x100 != 0 {
				r[35] = 1
			}
			gamestate.RowPut(r, 12+2*f.Station, 1)
		}
	case "remove":
		ordnanceRemove(r, c.Value)
	case "remove_all":
		for station := 0; station < 3; station++ {
			ordnanceRemove(r, station)
		}
		v.stack = v.stack[:1]
	case "replace":
		if int(int8(r[0])) != c.Value {
			row, err := gamestate.InitializeReadinessRow(v.Assets, v.Mission, c.Value)
			if err != nil {
				return false, err
			}
			v.Rows[v.Slot] = row
			v.Working = row
			r = v.Working[:]
			sh = v.definition()
		}
	case "back":
		if len(v.stack) > 1 {
			v.stack = v.stack[:len(v.stack)-1]
		} else {
			if message := ordnanceAcceptance(r, v.Slot); message != "" {
				v.audioCues = append(v.audioCues, gs4Denied)
				v.Message = message
				return false, nil
			}
			v.Rows[v.Slot] = v.Working
			v.Done = true
			return true, nil
		}
	}
	if err := gamestate.RecomputeReadinessRow(v.Assets, sh, r); err != nil {
		return false, err
	}
	v.rebuild()
	return false, nil
}

func ordnanceRemove(r []byte, station int) {
	r[3+station], r[18+station] = 255, 255
	gamestate.RowPut(r, 12+2*station, 0)
	if station == 2 {
		r[35] = 0
	}
}

func ordnanceAcceptance(r []byte, slot int) string {
	if gamestate.PreflightRatio(gamestate.RowWord(r, 25), gamestate.RowWord(r, 23)) > 100 {
		return "HELICOPTER IS LOADED BEYOND CAPACITY.  Decrease load."
	}
	if gamestate.RowWord(r, 29) == 0 {
		return "HELICOPTER HAS NO FUEL.  Add fuel."
	}
	armed := int8(r[2]) >= 0 && gamestate.RowWord(r, 21) > 0
	for station := 0; station < 3; station++ {
		armed = armed || gamestate.RowWord(r, 12+2*station) != 0
	}
	if slot == 0 && !armed {
		return "HELICOPTER IS UNARMED.  Add weapons."
	}
	return ""
}

func preflightComposeShip(a *assetdata.MissionAssets, id, scene int, row [36]byte) ([]byte, error) {
	ship, ok := a.Preflight.Ships[id]
	if !ok {
		return nil, fmt.Errorf("SHIP%d presentation missing", id)
	}
	backgroundScene := scene
	if scene == 5 {
		backgroundScene = 2
	}
	dst := bytes.Clone(a.Images[fmt.Sprintf("TARMC_%02d.PIC", backgroundScene)])
	if len(dst) != 64000 {
		return nil, fmt.Errorf("tarmac scene%d missing", backgroundScene)
	}
	src := assetdata.PreflightShipPixels(a, id, scene)
	draw := func(group int) error {
		active := false
		for _, c := range ship.Components {
			if c.Group != -1 {
				if active {
					break
				}
				active = c.Group == group
			}
			if active {
				if err := assetdata.OfficeBlit(src, dst, c.Source, c.Destination[0], c.Destination[1], true); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := draw(0); err != nil {
		return nil, err
	}
	sh := a.AircraftDefinitions[id]
	for station := 2; station >= 0; station-- {
		if gamestate.RowWord(row[:], 12+2*station) <= 0 {
			continue
		}
		choice := -1
		for i, f := range ship.Fits {
			q := int(int16(int(int8(sh[9])) * f.Quantity))
			if row[1] == 2 && station == 2 {
				q = f.Quantity
			}
			if f.Station == station && f.System == int(int8(row[3+station])) && q == int(gamestate.RowWord(row[:], 12+2*station)) {
				choice = i
			}
		}
		if choice >= 0 && ship.Fits[choice].Preview >= 0 {
			if err := draw(ship.Fits[choice].Preview); err != nil {
				return nil, err
			}
		}
	}
	return dst, nil
}

func ordnanceWrap(font *assetdata.BitmapFont, text string, width int) []string {
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			next := word
			if line != "" {
				next = line + " " + word
			}
			if font != nil && font.Measure([]byte(next)) > width && line != "" {
				result = append(result, line)
				line = word
			} else {
				line = next
			}
		}
		result = append(result, line)
	}
	return result
}

func preflightText(a *assetdata.MissionAssets, f []byte, selector int, text string, x, y, colour int, clip [4]int) {
	if font := a.Fonts[selector]; font != nil {
		_ = font.DrawClipped(f, []byte(text), x, y, colour, 0, 0, clip)
	}
}

func (v *OrdnanceView) Frame() []byte { return v.FrameForHit(v.Hover) }

func (v *OrdnanceView) FrameForHit(hover int) []byte {
	frame := make([]byte, 64000)
	drawPreflightFrame(v.Assets, frame, [4]int{0, 0, 226, 121}, 0, 0)
	drawPreflightFrame(v.Assets, frame, [4]int{0, 121, 226, 70}, 0, 0)
	drawPreflightFrame(v.Assets, frame, [4]int{226, 0, 94, 191}, 0, 0)
	// S0178 composes committed stores. S0716 changes only station markers;
	// the separate S1954 description remains responsive to the hovered row.
	art, err := preflightComposeShip(v.Assets, int(int8(v.Working[0])), v.Mission.Scene, v.Working)
	if err == nil {
		// S019a..01ad copies 219x114 at (3,3). The wider final native
		// transfer includes the frame; copying extra atlas columns adds a seam.
		_ = assetdata.OfficeBlit(art, frame, [4]int{0, 0, 219, 114}, 3, 3, false)
	}
	drawPreflightOutline(frame, [4]int{2, 2, 221, 116}, 9)
	v.drawStationMarkers(frame, hover)
	r, sh := v.Working[:], v.definition()
	flight := v.Flight[v.Slot]
	portrait := flight.Portrait
	if portrait < 0 || portrait >= 36 {
		portrait = 0
	}
	assetdata.OfficeFill(frame, 2, 123, 221, 7, 9)
	drawPreflightOutline(frame, [4]int{2, 131, 50, 34}, 9)
	_ = assetdata.OfficeBlit(v.Assets.Images["PILOT.PIC"], frame, [4]int{(portrait / 6) * 48, (portrait % 6) * 32, 48, 32}, 3, 132, false)
	preflightText(v.Assets, frame, 3, "CALLSIGN: "+flight.Callsign, 3, 124, 15, [4]int{3, 124, 219, 7})
	preflightText(v.Assets, frame, 3, "HELICOPTER TYPE: "+preflightCString(sh, 0x37), 53, 133, 10, [4]int{53, 132, 168, 14})
	group := "HEAVY"
	if flight.Group != 0 {
		group = "LIGHT"
	}
	leader := ""
	if flight.Leader && len(v.Rows) > 1 {
		leader = " LEADER"
	}
	name := assetdata.OfficeCString(flight.Raw[4:30])
	if name == "" {
		name = flight.Name
	}
	preflightText(v.Assets, frame, 3, name+"  "+group+" SECTION"+leader, 53, 139, 10, [4]int{53, 139, 168, 7})
	assetdata.OfficeFill(frame, 53, 146, 168, 1, 2)
	labels := []string{fmt.Sprintf("CHAFF: %d", gamestate.RowWord(r, 31)), fmt.Sprintf("FLARES: %d", 60-int(gamestate.RowWord(r, 31))), fmt.Sprintf("FUEL: %d %%", gamestate.PreflightRatio(gamestate.RowWord(r, 29), gamestate.RowWord(r, 27))), fmt.Sprintf("LOAD: %d %%", gamestate.PreflightRatio(gamestate.RowWord(r, 25), gamestate.RowWord(r, 23)))}
	if int8(r[2]) >= 0 {
		labels = append(labels, fmt.Sprintf("AMMO: %d rnds", gamestate.RowWord(r, 21)))
	}
	for i, s := range labels {
		preflightText(v.Assets, frame, 3, s, 54, 149+6*i, 10, [4]int{54, 148, 80, 42})
	}
	for station := 0; station < int(int8(sh[8])); station++ {
		y := 149 + 12*station
		preflightText(v.Assets, frame, 3, preflightCString(sh, 0x50+17*station)+":", 135, y, 10, [4]int{135, 148, 87, 43})
		text := "EMPTY"
		if n := gamestate.RowWord(r, 12+2*station); n > 0 {
			d := v.Assets.WeaponDefinitions[int(int8(r[3+station]))]
			text = fmt.Sprintf("%dx %s", n, preflightCString(d, 43))
		}
		preflightText(v.Assets, frame, 3, text, 137, y+6, 10, [4]int{135, 148, 87, 43})
	}
	v.drawDescription(frame, hover)
	for i, c := range v.Choices {
		if c.Action == "back" {
			drawPreflightFrame(v.Assets, frame, c.Rectangle, 1, 0)
			colour := 12
			if i == hover {
				colour = 15
			}
			if font := v.Assets.Fonts[3]; font != nil {
				preflightText(v.Assets, frame, 3, c.Text, c.Rectangle[0]+(c.Rectangle[2]-font.Measure([]byte(c.Text)))/2, c.Rectangle[1]+3, colour, c.Rectangle)
			}
			continue
		}
		colour := 10
		if c.Action == "heading" {
			colour = 2
		}
		if i == hover {
			colour = 15
		}
		for j, line := range ordnanceWrap(v.Assets.Fonts[3], c.Text, 76) {
			preflightText(v.Assets, frame, 3, line, 237, c.Rectangle[1]+3+j*6, colour, [4]int{229, 67, 91, 110})
		}
		if c.Action == "heading" {
			assetdata.OfficeFill(frame, 236, c.Rectangle[1]+c.Rectangle[3]-1, 79, 1, 2)
		}
	}
	message := v.Message
	if message == "" {
		message = ordnanceAcceptance(r, v.Slot)
	}
	if message == "" {
		message = v.text(0x40a8, "SELECTOR #1 to adjust Ordnance.  ACCEPT when final.")
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	if font := v.Assets.Fonts[3]; font != nil {
		ink := 7
		if ordnanceAcceptance(r, v.Slot) != "" {
			ink = 12
		}
		assetdata.DrawBottomTooltip(frame, font, message, ink)
	}
	return frame
}

func (v *OrdnanceView) drawStationMarkers(frame []byte, hover int) {
	if hover < 0 || hover >= len(v.Choices) || len(v.stack) == 0 {
		return
	}
	menu, choice := v.stack[len(v.stack)-1], v.Choices[hover]
	if (menu.Kind != "fits" && menu.Kind != "remove") || (choice.Action != "mount" && choice.Action != "remove") || choice.Station < 0 || choice.Station > 2 {
		return
	}
	if len(v.Assets.Images["POINTER.PIC"]) != 64000 {
		return
	}
	for corner, marker := range v.Assets.Preflight.StationMarkers {
		if corner >= 2 {
			break
		}
		at := 14 + 8*choice.Station + 4*corner
		x, y := int(int16(gamestate.RowWord(v.definition(), at))), int(int16(gamestate.RowWord(v.definition(), at+2)))
		if x == 0 {
			continue
		} // S0789/S07af gate only the anchor's X word.
		_ = assetdata.OfficeBlit(v.Assets.Images["POINTER.PIC"], frame, marker.Source, x-4, y-4, true)
	}
}

func (v *OrdnanceView) drawDescription(frame []byte, hover int) {
	icon, title, body := 85, "", ""
	bodyInk := 15
	choice := OrdnanceChoice{Action: "back"}
	if hover >= 0 && hover < len(v.Choices) {
		choice = v.Choices[hover]
	} else if menu := v.stack[len(v.stack)-1]; menu.Kind == "fits" || menu.Kind == "quantity" {
		choice = OrdnanceChoice{Action: "fits", System: menu.System}
	} else {
		for _, c := range v.Choices {
			if c.Action == "fits" {
				choice = c
				break
			}
		}
	}
	switch choice.Action {
	case "fits", "quantity_menu", "quantity", "mount", "remove":
		system := choice.System
		if choice.Action == "remove" && choice.Station >= 0 {
			system = int(int8(v.Working[3+choice.Station]))
		}
		if d := v.Assets.WeaponDefinitions[system]; len(d) == 214 {
			icon = system
			title = preflightCString(d, 68)
			body = preflightCString(d, 93)
		}
	case "replace":
		icon = 50 + choice.Value
		body = preflightCString(v.Assets.AircraftDefinitions[choice.Value], 131)
		// S08aa's payload byte8 selects description ink, independently of
		// S1742's normal10/highlight15 row inks. Rank denial never commits.
		if !choice.Enabled {
			bodyInk = 12
			body = v.text(0x3efb, "The Comanche is restricted, your rank must be 1st Lieut. or above.")
			if choice.Value == 1 {
				body = v.text(0x3eb5, "The Longbow Apache is restricted, your rank must be Captain or above.")
			}
		}
	case "aircraft":
		icon = 80
		body = v.text(0x3d57, "Change helicopter.")
	case "remove_menu", "remove_all":
		icon = 83
		body = v.text(0x3d6b, "Remove weapons.")
	}
	assetdata.OfficeFill(frame, 229, 3, 88, 65, 9)
	name, index := "MENU8085.PIC", icon-80
	if icon >= 0 && icon <= 21 {
		name, index = "MENU0021.PIC", icon
	}
	if icon >= 50 && icon <= 57 {
		name, index = "MENU5057.PIC", icon-50
	}
	if index >= 0 && index < 32 {
		pixels := v.Assets.Images[name]
		if (v.Mission.Scene == 4 || v.Mission.Scene == 5) && icon >= 30 && icon < 60 {
			pixels = bytes.Clone(pixels)
			for i, b := range pixels {
				pixels[i] = briefingIceIndex(b)
			}
		}
		_ = assetdata.OfficeBlit(pixels, frame, [4]int{index / 8 * 80, index % 8 * 25, 80, 25}, 233, 5, false)
	}
	preflightText(v.Assets, frame, 3, title, 233, 23, 0, [4]int{233, 6, 80, 62})
	for i, line := range ordnanceWrap(v.Assets.Fonts[3], body, 80) {
		preflightText(v.Assets, frame, 3, line, 233, 33+i*6, bodyInk, [4]int{233, 33, 80, 35})
	}
}
