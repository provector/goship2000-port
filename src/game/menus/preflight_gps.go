package menus

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/gpsref"
	"gunship2000-assets/internal/userpack"
	"math/big"
	"os"
	"path/filepath"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
)

var stockGPSCodes = gpsref.Default

type gpsCodes struct {
	Challenge     string `json:"challenge"`
	Authenticator string `json:"authenticator"`
}

type gpsChallenge struct {
	gpsCodes
	sprite int
}

type preflightGPSConfig struct {
	required bool
	choices  []gpsChallenge
	last     int // One-based, zero before the first draw.
}

type preflightGPS struct {
	initialized, locked, rejected bool
	choice                        gpsChallenge
	entry                         string
	shortcut                      int
}

func fiveDigits(s string) bool {
	if len(s) != 5 {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseGPSCodes(data []byte) ([]gpsChallenge, error) {
	var doc struct {
		Version int    `json:"schema_version"`
		Atlas   string `json:"atlas"`
		Logos   []struct {
			ID      string     `json:"id"`
			Name    string     `json:"name"`
			Sprite  *int       `json:"sprite"`
			Entries []gpsCodes `json:"entries"`
		} `json:"logos"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Version != 1 || doc.Atlas != "REAL_PCH.PIC" {
		return nil, fmt.Errorf("unsupported GPS catalogue version %d or atlas %q", doc.Version, doc.Atlas)
	}
	var choices []gpsChallenge
	ids := make(map[string]bool)
	sprites := make(map[int]bool)
	for _, logo := range doc.Logos {
		if logo.ID == "" || logo.Name == "" || ids[logo.ID] || logo.Sprite == nil || *logo.Sprite < 0 || *logo.Sprite >= 18 || sprites[*logo.Sprite] {
			return nil, fmt.Errorf("invalid or duplicate GPS insignia %q", logo.ID)
		}
		ids[logo.ID], sprites[*logo.Sprite] = true, true
		seen := make(map[string]bool)
		for _, pair := range logo.Entries {
			if !fiveDigits(pair.Challenge) || !fiveDigits(pair.Authenticator) || seen[pair.Challenge] {
				return nil, fmt.Errorf("invalid or repeated GPS code %q for %s", pair.Challenge, logo.ID)
			}
			seen[pair.Challenge] = true
			choices = append(choices, gpsChallenge{pair, *logo.Sprite})
		}
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("GPS catalogue has no selectable codes")
	}
	return choices, nil
}

// ConfigurePreflightGPS reads an optional editable catalogue once at startup.
func (c *FrontEnd) ConfigurePreflightGPS(required bool, directory string) {
	path := filepath.Join(directory, "gps_codes.json")
	var data []byte
	var err error
	if _, e := os.Lstat(filepath.Join(directory, userpack.ConfigName)); !os.IsNotExist(e) {
		path = filepath.Join(directory, userpack.ConfigName) + " / gps_codes.json"
		data, err = userpack.ReadConfig(directory, "gps_codes.json")
	} else {
		data, err = os.ReadFile(path)
		if os.IsNotExist(err) {
			data, err = stockGPSCodes, nil
		}
	}
	c.gps = preflightGPSConfig{required: required}
	if err == nil {
		c.gps.choices, err = parseGPSCodes(data)
	}
	if err != nil {
		diagnostics.Warn("preparation", "GPS catalogue unavailable", "path", path, "error", err)
	} else if !required {
		c.gps.choose() // Automatic lock uses one pair for the life of this game.
	}
}

func (config *preflightGPSConfig) choose() {
	count := len(config.choices)
	if count == 0 {
		return
	}
	skipPrevious := count > 1 && config.last != 0
	if skipPrevious {
		count--
	}
	draw, err := rand.Int(rand.Reader, big.NewInt(int64(count)))
	if err != nil {
		diagnostics.Warn("preparation", "GPS challenge selection failed", "error", err)
		config.last = 0
		return
	}
	index := int(draw.Int64())
	if skipPrevious && index >= config.last-1 {
		index++
	}
	config.last = index + 1
}

func (c *FrontEnd) enterPreflightGPS() {
	g := &c.Mission.gps
	if g.initialized {
		return
	}
	g.initialized = true
	g.locked = !c.gps.required
	if c.gps.required {
		c.gps.choose()
	}
	if c.gps.last != 0 {
		g.choice = c.gps.choices[c.gps.last-1]
		if g.locked {
			g.entry = g.choice.Authenticator
		}
	}
}

func (c *FrontEnd) gpsAllowsStart() bool {
	return !c.gps.required || c.Mission.gps.locked
}

func (c *FrontEnd) gpsReject(message string) {
	c.emitAudioCue(gs4Denied)
	c.Mission.PreflightWarning = message
}

// Original 1f13:036b..03d8 decodes a three-column keypad. Restrict hits
// to the painted grid before division so the adjacent radio bezel is inert.
func preflightKeyAt(x, y int) rune {
	if x < 192 || x >= 225 || y < 87 || y >= 119 {
		return 0
	}
	return rune("123456789#0E"[((y-87)/8)*3+(x-192)/11])
}

func (c *FrontEnd) handleGPSInput(a Actions) {
	g := &c.Mission.gps
	editable := !g.locked && g.choice.Challenge != ""
	press := func(key rune) {
		if key == 0 {
			return
		}
		// The PC uses GS4 13h for a physical digit. Use the same mechanical
		// feedback for deliberate key presses, independently of the lock.
		c.emitAudioCue(gs4Knob)
		if g.locked || !editable || key == '#' {
			return
		}
		c.Mission.PreflightWarning = ""
		switch key {
		case 'E':
			g.rejected = false
			g.entry = ""
		case '\b':
			g.rejected = false
			if len(g.entry) > 0 {
				g.entry = g.entry[:len(g.entry)-1]
			}
		default:
			if g.rejected {
				g.entry = ""
				g.rejected = false
			}
			g.entry += string(key)
			if len(g.entry) == 5 {
				if g.entry == g.choice.Authenticator {
					g.locked = true
					c.emitAudioCue(gs4Accepted)
				} else {
					g.rejected = true
					c.gpsReject("GPS code rejected. Enter five digits to retry.")
				}
			}
		}
	}
	if a.Selector1 {
		if key := preflightKeyAt(a.X, a.Y); key != 0 {
			if key != '#' {
				g.shortcut = 0
			}
			press(key)
		}
	}
	if !editable || g.locked {
		return
	}
	if a.Backspace {
		g.shortcut = 0
		press('\b')
	}
	for _, key := range a.Text {
		if g.locked {
			break
		}
		if key == 'e' {
			key = 'E'
		}
		if key == '#' {
			press(key)
		} else if key == 'E' || key >= '0' && key <= '9' {
			g.shortcut = 0
			press(key)
		}
	}
	for _, step := range a.gpsSteps {
		if g.locked {
			break
		}
		if step == 1 {
			g.entry = ""
			g.rejected = false
			g.shortcut = 0
		}
		if step != g.shortcut+1 || step < 1 || step > 5 {
			continue
		}
		g.shortcut = step
		press(rune(g.choice.Authenticator[step-1]))
	}
}

func (c *FrontEnd) drawRadioLCD(frame []byte) {
	a := c.Office.Assets.Missions
	g := c.Mission.gps
	locked := c.gpsAllowsStart()
	// REALITY.SCN LORI 0 and 1. The PC entry draws the two digit rows
	// at x=150..174 (six-pixel advance) and the annunciator at 120,97.
	sprite := func(key, index, x, y int) {
		for _, l := range a.Metadata.Screens["REALITY.SCN"].Lori {
			if l.Key != key || l.Divisor <= 0 {
				continue
			}
			r := [4]int{l.Base[0] + index/l.Divisor*l.Cell[0], l.Base[1] + index%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
			_ = assetdata.OfficeBlit(a.Images["REALI_AN.PIC"], frame, r, x, y, l.Mode != 0)
			return
		}
	}
	lamp := 0
	if locked {
		lamp = 1
	}
	sprite(1, lamp, 120, 97)
	for row, digits := range []string{g.choice.Challenge, g.entry} {
		for i := 0; i < 5; i++ {
			idx := 10 // Unlit seven-segment glyph.
			if i < len(digits) {
				idx = int(digits[i] - '0')
			}
			sprite(0, idx, 150+i*6, 90+row*9)
		}
	}
}

func (c *FrontEnd) drawGPSMonitor(frame []byte) {
	a := c.Office.Assets.Missions
	g := c.Mission.gps
	locked := c.gpsAllowsStart()
	clip := [4]int{200, 8, 100, 56}
	center := clip[0] + clip[2]/2
	if g.choice.Challenge != "" {
		i := g.choice.sprite
		_ = assetdata.OfficeBlit(a.Images["REAL_PCH.PIC"], frame, [4]int{i % 4 * 47, i / 4 * 39, 47, 39}, clip[0]+(clip[2]-47)/2, clip[1], false)
	}
	if font := a.Fonts[2]; font != nil {
		line := func(text string, y int) {
			_ = font.DrawClipped(frame, []byte(text), center-font.Measure([]byte(text))/2, y, 41, 0, 0, clip)
		}
		if g.choice.Challenge == "" {
			if locked {
				line("Signal Locked", 42)
			} else {
				line("Codes unavailable", 30)
			}
			return
		}
		// The full 39-row patch and both text lines fit with two clear rows between each.
		line(g.choice.Challenge, 49)
		if locked {
			line("Signal Locked", 57)
		}
	}
}
