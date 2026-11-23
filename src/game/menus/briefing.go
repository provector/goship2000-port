package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"math"
	"math/rand"
	"strings"
)

// BriefingView owns presentation effects only. Its independent RNG never changes
// the generated mission, and page switches reuse the same accepted population.
type BriefingView struct {
	Assets               *assetdata.MissionAssets
	Mission              *gamestate.MissionData
	Pilot                *gamestate.Pilot
	scene, clean         []byte
	effects              *rand.Rand
	commander            commanderController
	page                 int
	visibilityDraw       int
	commanderPacingPhase uint8
}

type commanderController struct {
	Counter                          byte
	State, Repeat, Index, Head, Pose int
	Path                             []int
}

func NewBriefingView(a *assetdata.MissionAssets, m *gamestate.MissionData, p *gamestate.Pilot, seed int64) (*BriefingView, error) {
	if a == nil || m == nil || p == nil {
		return nil, fmt.Errorf("briefing requires prepared assets, mission and selected pilot")
	}
	if len(a.Images[m.StableScene+".PIC"]) != 64000 {
		return nil, fmt.Errorf("briefing image %s missing", m.StableScene)
	}
	if a.Fonts[2] == nil || a.Fonts[3] == nil {
		return nil, fmt.Errorf("briefing requires bitmap fonts2 and3")
	}
	v := &BriefingView{Assets: a, Mission: m, Pilot: p, effects: rand.New(rand.NewSource(seed)), page: -1}
	v.clean = append([]byte(nil), a.Images[m.StableScene+".PIC"]...)
	if m.Inputs.MissionType == gamestate.MissionCampaign {
		if len(a.Metadata.CampaignStatus) != 7 {
			return nil, fmt.Errorf("Campaign paper requires prepared status text; prepare25")
		}
		if err := v.drawCampaignIllustration(); err != nil {
			diagnostics.Warn("briefing.illustration", "Optional Campaign illustration omitted", "scene", m.StableScene, "cause", err)
		}
	}
	// FIELD's two scenery cells are placed behind the commander before its restore
	// region is captured. Ship/ice scenes do not take this land-scene LORI path.
	if m.Scene == 0 || m.Scene == 1 {
		if s, ok := a.Metadata.Screens[m.StableScene+".SCN"]; ok {
			for _, l := range s.Lori {
				if l.Key == 1 && l.Divisor > 0 {
					r := [4]int{l.Base[0] + m.Scene/l.Divisor*l.Cell[0], l.Base[1] + m.Scene%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
					_ = assetdata.OfficeBlit(a.Images[s.Basenames[1]+".PIC"], v.clean, r, 198, 58, true)
				}
			}
		}
	}
	v.scene = append([]byte(nil), v.clean...)
	v.advanceCommander(true)
	v.SetPaper(0)
	return v, nil
}

func (v *BriefingView) anchor() (int, int) {
	if v.Mission.Scene == 2 || v.Mission.Scene == 4 || v.Mission.Scene == 5 {
		return 147, 74
	}
	return 126, 35
}

func (v *BriefingView) SetPaper(page int) {
	if page < 0 || page > 3 {
		return
	}
	if v.page != page {
		v.page = page
		if page == 1 {
			v.visibilityDraw = v.effects.Intn(2)
		}
	}
}

// This is a native animation adjustment,
// not a recovered DOS delay: three commander visits per four UI ticks (75%).
// It affects neither prebrief/page/fade clocks nor authoritative mission RNG.

func (v *BriefingView) Tick() {
	v.commanderPacingPhase += 3
	if v.commanderPacingPhase < 4 {
		return
	}
	v.commanderPacingPhase -= 4
	v.advanceCommander(false)
}

func (v *BriefingView) advanceCommander(forced bool) {
	c := &v.commander
	previous := c.Counter
	c.Counter++
	if forced || previous > 12 {
		c.Counter = 0
		if c.Repeat <= 0 {
			c.Repeat = 5 + v.effects.Intn(11)
			c.Index = 0
			old := c.State
			if old == 0 || (old == 1 && v.effects.Intn(3) != 0) {
				next := 0
				if old == 0 {
					next = 1
				}
				c.Path = []int{next}
				c.State = next
			} else {
				next := 1 + v.effects.Intn(2)
				if old <= next {
					next++
				}
				key := fmt.Sprintf("%d->%d", old, next)
				c.Path = append([]int(nil), v.Assets.Metadata.Transitions[key]...)
				c.State = next
			}
		}
		if len(c.Path) == 0 {
			c.Path = []int{0}
		}
		c.Pose = c.Path[min(c.Index, len(c.Path)-1)]
		c.Head = v.effects.Intn(5)
		v.drawCommander()
		if c.Index < len(c.Path)-1 {
			c.Index++
		} else {
			c.Repeat--
		}
	}
	// The original skipped-main branch reads an uninitialized local word. Retain
	// the last selected head explicitly, rather than pretending DOS stack residue
	// is deterministic across calls. This policy affects facial detail only.
	if c.Head == 2 {
		v.drawFace(v.effects.Intn(9))
	}
}

func (v *BriefingView) drawCommander() {
	ax, ay := v.anchor()
	_ = assetdata.OfficeBlit(v.clean, v.scene, [4]int{ax - 73, ay - 15, 125, 90}, ax-73, ay-15, false)
	poses := v.Assets.Metadata.Poses
	if v.commander.Pose < 0 || v.commander.Pose >= len(poses) {
		return
	}
	p := poses[v.commander.Pose]
	name := "COMMANDR.PIC"
	if v.Mission.Scene == 2 || v.Mission.Scene == 4 || v.Mission.Scene == 5 {
		name = "COMMAND2.PIC"
	}
	sheet := v.Assets.Images[name]
	_ = assetdata.OfficeBlit(sheet, v.scene, p.Body, ax-28, ay+8, true)
	_ = assetdata.OfficeBlit(sheet, v.scene, [4]int{v.commander.Head * 32, 0, 32, 32}, ax-15, ay-15, true)
	dx, dy := ax-52, ay+13
	if p.Triple[1] != 0 {
		dx, dy = ax-73, ay+16
	}
	_ = assetdata.OfficeBlit(sheet, v.scene, p.Arms, dx, dy, true)
}

func (v *BriefingView) drawFace(n int) {
	ax, ay := v.anchor()
	name := "COMMANDR.PIC"
	if v.Mission.Scene == 2 || v.Mission.Scene == 4 || v.Mission.Scene == 5 {
		name = "COMMAND2.PIC"
	}
	_ = assetdata.OfficeBlit(v.Assets.Images[name], v.scene, [4]int{274, 88 + n*6, 13, 6}, ax-6, ay+7, false)
}

func (v *BriefingView) Frame() []byte {
	f := append([]byte(nil), v.scene...)
	caption := []string{"MISSION ORDERS", "S-2", "MISSION MAP", "DECLINE MISSION"}[max(0, v.page)]
	font := v.Assets.Fonts[2]
	_ = font.Draw(f, []byte(caption), (227-font.Measure([]byte(caption)))/2, 180, 0, 0, 0)
	assetdata.OfficeFill(f, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(f, v.captionFont(), "MISSION ORDERS", 7)
	return f
}

func (v *BriefingView) Hit(x, y int) int {
	s, ok := v.Assets.Metadata.Screens[v.Mission.StableScene+".SCN"]
	if !ok {
		return -1
	}
	for i := len(s.Areas) - 1; i >= 0; i-- {
		a := s.Areas[i]
		r := a.Rectangle
		if x >= r[0] && y >= r[1] && x < r[0]+r[2] && y < r[1]+r[3] {
			return a.Key
		}
	}
	return -1
}

func (v *BriefingView) InterstitialName() string {
	if v.Mission.Scene == 4 {
		return "ICE"
	}
	if v.Mission.Scene == 2 || v.Mission.Scene == 5 {
		return "SHIP"
	}
	return "TENT"
}

func (v *BriefingView) InterstitialFrame() []byte {
	pixels := append([]byte(nil), v.Assets.Images[v.InterstitialName()+".PIC"]...)
	// A1e20e transforms the pre-tent SHIP bank for ice-carrier scene5.
	// The separate A25b61 bank3 operation belongs to aircraft/loadout data,
	// not the commander atlas; immutable prepared indices remain unchanged.
	if v.Mission.Scene == 5 {
		for i, p := range pixels {
			pixels[i] = briefingIceIndex(p)
		}
	}
	return pixels
}

// briefingFormat substitutes only source format arguments. Colour toggles and
// tab controls remain in the stream for the original bitmap paragraph layout.
func briefingFormat(format string, args ...any) string {
	var out strings.Builder
	at := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}
		if format[i] == 'c' || format[i] == 't' {
			out.WriteByte('%')
			out.WriteByte(format[i])
			continue
		}
		start := i
		for i < len(format) && (format[i] == '0' || format[i] >= '1' && format[i] <= '9') {
			i++
		}
		if i >= len(format) {
			break
		}
		if at >= len(args) {
			out.WriteString("?")
			continue
		}
		out.WriteString(fmt.Sprintf("%"+format[start:i+1], args[at]))
		at++
	}
	return out.String()
}

func (v *BriefingView) template(ds int, args ...any) string {
	return briefingFormat(v.Assets.Metadata.Templates[ds], args...)
}

func (v *BriefingView) grid(p gamestate.MissionPoint) string {
	return fmt.Sprintf("%02d/%02d", gamestate.MissionGrid(p.X), gamestate.MissionGrid(p.Y))
}

func (v *BriefingView) objective(o gamestate.MissionObjective, primary bool) string {
	grid := v.grid(o.Point)
	name := o.Name
	if o.Endpoint != nil && o.Endpoint.X != 0 {
		dx, dy := int32(o.Endpoint.X-o.Point.X), int32(o.Endpoint.Y-o.Point.Y)
		if dx != 0 || dy != 0 {
			heading := uint16(int64(math.Atan2(float64(dx), float64(dy)) * math.Float64frombits(0x3fc45f318e7adaf5) * 65535))
			labels := v.Assets.Generation.Tables.Compass
			sector := (((heading >> 8) + 8) & 0xf0) >> 4
			if int(sector) < len(labels) {
				return v.template(0x36eb, name, labels[sector], grid)
			}
		}
	}
	// A18d7e returns the raw DS:b5d0 objective byte to papers P0a52..0c12.
	// DisplayKind is a separate mapped category used by population/loadout consumers.
	switch o.RawKind {
	case 3:
		return v.template(0x3732, grid, v.template(0x372e, name))
	case 7:
		return v.template(0x37be, grid)
	case 1:
		return v.template(0x3817, name, grid)
	case 6:
		return v.template(0x385b, name, v.template(0x3857, grid))
	case 4:
		return v.template(0x3893, grid, v.template(0x388f, name))
	case 5:
		return v.template(0x38be, name, v.template(0x38ba, grid))
	case 2:
		if primary {
			base := v.Mission.Objectives["base"]
			x, y := gamestate.MissionGrid(base.Point.X), gamestate.MissionGrid(base.Point.Y)
			edge := 3
			n := x
			switch {
			case x <= 2:
				edge = 2
				n = y
			case x >= 14:
				edge = 0
				n = y
			case y <= 2:
				edge = 1
			}
			edgeNames := []string{v.template(0x36cb), v.template(0x36d0), v.template(0x36d6), v.template(0x36db)}
			corridor := v.template(0x3762, edgeNames[edge], n-2, n+2)
			return v.template(0x3776, corridor, grid, v.template(0x3772, name))
		}
	}
	return v.template(0x38e2)
}

func (v *BriefingView) OrdersText() string {
	m := v.Mission
	t := v.Assets.Generation.Tables
	result := v.template(0x335f, m.Conditions.Operation)
	if m.Inputs.MissionType == gamestate.MissionTraining {
		result = v.template(0x3344)
	}
	rank := ""
	if n := v.Pilot.Rank.Raw(); n >= 0 && n < len(t.Ranks) {
		rank = t.Ranks[n]
	}
	name := v.Pilot.Name
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[len(fields)-1]
	}
	result += v.template(0x3370, rank, name) + v.template(0x3395, fmt.Sprintf("%02d", m.Conditions.Hour))
	if i := m.Conditions.Callsign; i >= 0 && i < len(t.Callsigns) {
		result += v.template(0x33a8, t.Callsigns[i])
	}
	if m.Inputs.MissionType == gamestate.MissionCampaign {
		result += v.campaignStatusText()
	}
	for _, role := range []string{"primary", "secondary"} {
		if o, ok := m.Objectives[role]; ok && o.DisplayKind != 0 {
			ds := 0x34a9
			if role == "secondary" {
				ds = 0x34bc
			}
			result += v.template(ds) + v.objective(o, role == "primary")
		}
	}
	if o, ok := m.Objectives["base"]; ok {
		result += v.template(0x34db, v.grid(o.Point))
	}
	if o, ok := m.Objectives["FARP"]; ok {
		result += v.template(0x34f5, v.grid(o.Point))
	}
	return result
}

func (v *BriefingView) IntelligenceText() string {
	m := v.Mission
	c := m.Conditions
	t := v.Assets.Generation.Tables
	day := v.template(0x350f)
	if c.Flags&0x200 != 0 {
		day = v.template(0x3509)
		if m.Scene == 4 || m.Scene == 5 {
			day = v.template(0x3504)
		}
	}
	result := v.template(0x3513, day)
	if c.WindSpeed != 0 {
		sector := (((c.WindDirection >> 8) + 8) & 0xf0) >> 4
		compass := ""
		if int(sector) < len(t.Compass) {
			compass = t.Compass[sector]
		}
		result += v.template(0x352f, int(float64(c.WindDirection)*t.WindMultiplier), compass, c.WindSpeed)
	}
	result += v.template(0x3577, t.VisibilityPairs[(c.Flags>>8)&1][v.visibilityDraw]) + v.template(0x3597, v.template(0x3591, int(c.Temperature)))
	intel := m.Intelligence
	if title := v.friendlySupportTitle(); title != "" {
		result += v.template(0x35b3, title)
	}
	if len(intel.GroundNames) > 0 {
		names := ""
		for i, n := range intel.GroundNames {
			if i > 0 {
				if i == len(intel.GroundNames)-1 {
					names += v.template(0x35f8)
				} else {
					names += v.template(0x35fe)
				}
			}
			names += n
		}
		result += v.template(0x3601, names)
	}
	if intel.AirCount > 0 {
		result += v.template(0x3667)
		if intel.AirCount < 4 {
			result += v.template(0x368d)
		} else {
			if intel.AirCount > 5 {
				result += v.template(0x3694)
			}
			result += v.template(0x369f)
		}
		result += v.template(0x36a6, intel.AirName)
	}
	return result
}

// P0616..0635 prints the S-2 Support paragraph only when A270 is active;
// 065c:2cb6 obtains its formation title. Generated missions already carried
// that title. Authored/frozen realizations carry the equivalent actual Support
// selection instead. This is presentation of immutable mission inputs only.
func (v *BriefingView) friendlySupportTitle() string {
	m := v.Mission
	if m.Support == nil {
		return m.Intelligence.FriendlyTitle // Explicit historical presentation carrier.
	}
	if m.Support.Mode == 0 {
		return ""
	}
	w := m.SourceWorld
	fi := m.Support.Selection.Formation
	if w == nil || fi < 0 || fi >= len(w.FMHD)/39 {
		return ""
	}
	return strings.SplitN(string(w.FMHD[fi*39:fi*39+25]), "\x00", 2)[0]
}

// PaperFrame reconstructs the paper work surface, never a captured framebuffer.
// The two HANDS strips are the original page shading copied at P14f3..1512.
func (v *BriefingView) PaperFrame(page int) []byte {
	return v.paperFrame(page, true)
}

func (v *BriefingView) paperFrame(page int, hand bool) []byte {
	if page < 0 || page > 3 {
		page = 0
	}
	f := append([]byte(nil), v.Assets.Images["PAGE.PIC"]...)
	if v.Mission.Scene == 2 || v.Mission.Scene == 5 {
		f = append([]byte(nil), v.Assets.Images["PAGE2.PIC"]...)
	} else if v.Mission.Scene == 4 {
		f = append([]byte(nil), v.Assets.Images["PAGE3.PIC"]...)
	}
	if len(f) != 64000 {
		f = make([]byte, 64000)
	}
	assetdata.OfficeFill(f, 0, 0, 227, 191, 15)
	for _, y := range []int{33, 112} {
		_ = assetdata.OfficeBlit(v.Assets.Images["HANDS.PIC"], f, [4]int{0, 154, 227, 46}, 0, y, false)
	}
	title := []string{"MISSION ORDERS", "S-2", "MISSION MAP", "DECLINE MISSION"}[page]
	font := v.Assets.Fonts[2]
	titleWidth := 207
	if page < 2 {
		titleWidth = 187
	}
	_ = font.Draw(f, []byte(title), (titleWidth-font.Measure([]byte(title)))/2+10, 8, 0, 0, 0)
	switch page {
	case 0:
		v.paragraph(f, v.OrdersText(), 10, 24, 187, 191, 50, 20)
	case 1:
		v.paragraph(f, v.IntelligenceText(), 10, 24, 187, 191, 50, 20)
	case 2:
		v.drawMap(f)
	case 3:
		text := v.declineText()
		y := v.paragraph(f, text, 10, 24, 207, 171, 100, 10)
		v.drawDeclineChoice(f, y)
	}
	if hand {
		v.drawPaperHand(f)
	}
	assetdata.OfficeFill(f, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(f, v.captionFont(), "VIEW NEXT PAGE", 7)
	return f
}

func (v *BriefingView) drawPaperHand(frame []byte) {
	v.drawPaperHandOffset(frame, 0)
}

func (v *BriefingView) drawPaperHandOffset(frame []byte, offset int) {
	// P0225's hand is a foreground sprite, not horizontally scrolling prose.
	// Vertical transitions move it with the right-side artwork and clip before
	// the stationary footer; the panel's leading header must not split the hand.
	y := 134 + offset
	if height := min(57, paperViewHeight-y); height > 0 {
		_ = assetdata.OfficeBlit(v.Assets.Images["HANDS.PIC"], frame, [4]int{249, 0, 71, height}, 199, y, true)
	}
}

func (v *BriefingView) drawDeclineChoice(f []byte, y int) {
	for _, choice := range v.declineChoices(y) {
		outline(f, [4]int{choice.Rectangle[0], choice.Rectangle[1], 8, 8}, 0)
		_ = v.Assets.Fonts[2].Draw(f, []byte(v.template(choice.TextDS)), 22, choice.Rectangle[1]+1, 0, 0, 0)
	}
}

func (v *BriefingView) declineText() string {
	if v.Mission.Inputs.MissionType == gamestate.MissionTraining {
		return v.template(0x38f8)
	}
	text := v.template(0x3970)
	if v.Mission.Inputs.MissionType == gamestate.MissionCampaign {
		text += v.template(0x3b3f)
	}
	return text + v.template(0x3b88)
}

type paperChoice struct {
	Key, TextDS int
	Rectangle   [4]int
}

func (v *BriefingView) declineChoices(y int) []paperChoice {
	ids := []int{0x3bb9}
	if v.Mission.Inputs.MissionType == gamestate.MissionCampaign {
		ids = append(ids, 0x3bc9, 0x3bda)
	}
	// P0d22..0d36 places generated actions ten logical rows apart. Bound
	// this port's wrapping cursor so the last action remains above the footer.
	y = min(y, 184-10*(len(ids)-1))
	choices := make([]paperChoice, len(ids))
	for i, ds := range ids {
		choices[i] = paperChoice{5 + i, ds, [4]int{10, y + 10*i, 100, 10}}
	}
	return choices
}

// PaperHit keeps RETURN and next-page areas separate. The generated Decline
// option uses its paragraph cursor, rather than a guessed fixed screen row.
var paperZones = []assetdata.OfficeArea{
	{Key: 3, Rectangle: [4]int{0, 0, 227, 191}},
	{Key: 4, Rectangle: [4]int{220, 0, 100, 192}},
}

func (v *BriefingView) paperAreas(page int) []assetdata.OfficeArea {
	if page != 3 {
		return paperZones
	}
	cy := v.paragraph(make([]byte, 64000), v.declineText(), 10, 24, 207, 171, 100, 10)
	areas := append([]assetdata.OfficeArea(nil), paperZones...)
	for _, choice := range v.declineChoices(cy) {
		areas = append(areas, assetdata.OfficeArea{Key: choice.Key, Rectangle: choice.Rectangle})
	}
	return areas
}

func (v *BriefingView) PaperHit(page, x, y int) int {
	// The exposed right-hand RETURN strip always wins the paper overlap.
	if insideRect(x, y, paperZones[1].Rectangle) {
		return 4
	}
	areas := v.paperAreas(page)
	for i := len(areas) - 1; i >= 0; i-- {
		if insideRect(x, y, areas[i].Rectangle) {
			return areas[i].Key
		}
	}
	return -1
}

// The game's formatter changes colour without adding a character advance.
// Paragraph layout preserves source whitespace, tabs and word boundaries;
// unlike a host text widget it uses the prepared bitmap's exact advances.
func (v *BriefingView) paragraph(dst []byte, text string, left, top, width, bottom, firstTab, tabSpacing int) int {
	font := v.Assets.Fonts[2]
	x, y := left, top
	colour := 0
	line := font.Metrics.Height + font.Metrics.Spacing
	if line < 8 {
		line = 8
	}
	clip := [4]int{left, top, width, max(0, bottom-top)}
	newline := func() { x = left; y += line }
	for i := 0; i < len(text); {
		switch text[i] {
		case '\n':
			newline()
			i++
			continue
		case '\r':
			i++
			continue
		}
		if text[i] == '%' && i+1 < len(text) {
			switch text[i+1] {
			case 'c':
				if colour == 0 {
					colour = 4
				} else {
					colour = 0
				}
				i += 2
				continue
			case 't':
				target := left + firstTab
				if x >= target && tabSpacing > 0 {
					target += ((x-target)/tabSpacing + 1) * tabSpacing
				}
				x = target
				i += 2
				continue
			}
		}
		if text[i] != ' ' {
			end := i
			var visible strings.Builder
			for end < len(text) && text[end] != ' ' && text[end] != '\n' && text[end] != '\r' {
				if text[end] == '%' && end+1 < len(text) && (text[end+1] == 'c' || text[end+1] == 't') {
					end += 2
					continue
				}
				visible.WriteByte(text[end])
				end++
			}
			if x > left && x+font.Measure([]byte(visible.String())) > left+width {
				newline()
			}
		}
		b := text[i]
		if x+font.Measure([]byte{b}) > left+width {
			newline()
		}
		if y < bottom {
			_ = font.DrawClipped(dst, []byte{b}, x, y, colour, 0, 0, clip)
		}
		x += font.Measure([]byte{b})
		i++
	}
	return y
}

func (v *BriefingView) drawMap(f []byte) {
	_ = assetdata.OfficeBlit(v.Assets.Images[v.Mission.Map], f, [4]int{0, 0, 166, 132}, 17, 18, false)
	// The source legend is in the map sheet below its132-row display region;
	// HANDS has only zero bytes here. Selector numbers at the extraction jump
	// must not be substituted for the SCN loader's bank labels.
	_ = assetdata.OfficeBlit(v.Assets.Images[v.Mission.Map], f, [4]int{0, 132, 165, 22}, 17, 152, true)
	for _, entry := range []struct{ role, label string }{{"primary", "P"}, {"secondary", "S"}, {"base", "B"}, {"FARP", "F"}} {
		o, ok := v.Mission.Objectives[entry.role]
		if !ok {
			continue
		}
		x, y := gamestate.MissionMapPoint(o.Point, 0, 0)
		if entry.role == "base" {
			if line, ok := briefingMapEdgeSegment(o.Point, v.Mission.Conditions.Flags); ok {
				_ = assetdata.OfficeLine(f, line[0], line[1], line[2], line[3], 9)
			}
		}
		// The source renders a black offset shadow before its blue/white marker.
		v.markerFill(f, x-1, y, 6, 7, 0)
		_ = v.Assets.Fonts[3].Draw(f, []byte(entry.label), x, y+1, 0, 0, 0)
		if o.Endpoint != nil && o.Endpoint.X != 0 {
			ex, ey := gamestate.MissionMapPoint(*o.Endpoint, 0, 0)
			ex++ // Source endpoint uses +20; the label/start anchor uses +19.
			briefingArrow(f, x+3, y+3, ex+3, ey+3, 0)
			briefingArrow(f, x+2, y+2, ex+2, ey+2, 14)
		}
		v.markerFill(f, x-2, y-1, 6, 7, 1)
		_ = v.Assets.Fonts[3].Draw(f, []byte(entry.label), x-1, y, 15, 0, 0)
	}
}

func (v *BriefingView) markerFill(f []byte, x, y, w, h int, c byte) {
	for yy := max(0, y); yy < min(200, y+h); yy++ {
		for xx := max(0, x); xx < min(320, x+w); xx++ {
			f[yy*320+xx] = c
		}
	}
}

func briefingArrow(f []byte, x, y, ex, ey int, c byte) {
	for _, line := range briefingMapArrowSegments(x, y, ex, ey) {
		_ = assetdata.OfficeLine(f, line[0], line[1], line[2], line[3], c)
	}
}

func (v *BriefingView) Caption(key int) string {
	switch key {
	case 1:
		return "OUTFIT SUMMARY"
	case 2:
		return "MISSION ORDERS"
	case 3:
		return "BEGIN MISSION"
	}
	return ""
}

func (v *BriefingView) FrameForHit(key int) []byte {
	f := v.Frame()
	assetdata.OfficeFill(f, 0, 191, 320, 9, 0)
	if caption := v.Caption(key); caption != "" {
		assetdata.DrawBottomTooltip(f, v.captionFont(), caption, 7)
	}
	return f
}

func (v *BriefingView) PaperFrameForHit(page, key int) []byte {
	f := v.PaperFrame(page)
	caption := "VIEW NEXT PAGE"
	if key == 4 {
		caption = "RETURN"
	}
	if key >= 5 && key <= 7 && page == 3 {
		caption = ""
		text := v.declineText()
		y := v.paragraph(make([]byte, 64000), text, 10, 24, 207, 171, 100, 10)
		for _, choice := range v.declineChoices(y) {
			if choice.Key == key {
				outline(f, [4]int{choice.Rectangle[0], choice.Rectangle[1], 8, 8}, 9)
			}
		}
	}
	assetdata.OfficeFill(f, 0, 191, 320, 9, 0)
	assetdata.DrawBottomTooltip(f, v.captionFont(), caption, 7)
	return f
}

func briefingIceIndex(p byte) byte {
	if p >= 0x45 && p <= 0x4f {
		return byte((20*int(p)-0x564)/11 - 0x18)
	}
	if p >= 0xc0 && p <= 0xdf {
		return p + 0x20
	}
	return p
}

func (v *BriefingView) captionFont() *assetdata.BitmapFont {
	if f := v.Assets.Fonts[5]; f != nil {
		return f
	}
	return v.Assets.Fonts[3]
}

// EnterPapers starts a presentation pass even when the remembered page did not
// change. Native Intel wording draws again on entry, but not on host refresh.
func (v *BriefingView) EnterPapers(page int) { v.page = -1; v.SetPaper(page) }

// ResumeScene mirrors B005c's rebuild after a paper/settings round trip. The
// animation counters survive; this is a forced draw, not mission regeneration.
func (v *BriefingView) ResumeScene() { copy(v.scene, v.clean); v.advanceCommander(true) }
