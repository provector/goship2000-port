package assets

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"sort"
	"strings"
)

type cockpitMetadata struct {
	Schema  int
	Profile string
	Data    cockpit.Assets
	Files   map[string]Output
	Sources []SourceLocation
	Slices  []cockpitSliceProvenance
	Notes   []string
}
type cockpitSliceProvenance struct {
	Name                                string
	Section, CS, IP, FileOffset, Length int
	SHA256                              string
}

func cockpitSourceBytes(b []byte, l *CockpitSourceLayout, section, cs, ip, n int) ([]byte, int, error) {
	if n < 0 || ip < 0 || ip > 65535 {
		return nil, 0, fmt.Errorf("invalid cockpit source range")
	}
	at := 0
	if section < 0 {
		at = 0x600 + 16*cs + ip
		if at+n > 0x9d4f {
			return nil, 0, fmt.Errorf("cockpit resident range outside image")
		}
	} else {
		if section >= len(l.SectionBodies) {
			return nil, 0, fmt.Errorf("invalid cockpit source section")
		}
		at = l.SectionBodies[section] + 16*(cs-l.SectionCS[section]) + ip
		end := len(b)
		if section+1 < len(l.SectionBodies) {
			end = l.SectionBodies[section+1]
		}
		if at < l.SectionBodies[section] || at+n > end {
			return nil, 0, fmt.Errorf("cockpit source range outside section%d", section)
		}
	}
	if at < 0 || at+n > len(b) {
		return nil, 0, fmt.Errorf("truncated cockpit source")
	}
	return b[at : at+n], at, nil
}

// DecodeF5 implements the admitted count-terminated signed-width LZW variant.
// Its first byte is not F505 magic; masking belongs to the later composition.
func DecodeF5(raw []byte, size int) ([]byte, error) {
	if len(raw) == 0 || raw[0] < 0xf5 || raw[0] > 0xf7 || size < 1 || size > 65535 {
		return nil, fmt.Errorf("unsupported F5 width/output count")
	}
	maximum := 256 - int(raw[0])
	width, next, previous, first := 9, 256, -1, -1
	bit := 8
	var prefix [2048]int16
	var suffix [2048]byte
	for i := range prefix {
		prefix[i] = -1
		suffix[i] = byte(i)
	}
	out := make([]byte, 0, size)
	stack := make([]byte, 0, 2048)
	for len(out) < size {
		if bit+width > len(raw)*8 {
			return nil, fmt.Errorf("F5 missing consumed bits at%d, output%d", bit, len(out))
		}
		code := 0
		for k := 0; k < width; k++ {
			code |= int((raw[(bit+k)/8]>>((bit+k)&7))&1) << k
		}
		bit += width
		original := code
		stack = stack[:0]
		if code == next {
			if first < 0 {
				return nil, fmt.Errorf("F5 first next-slot reads uninitialized state")
			}
			stack = append(stack, byte(first))
			code = previous
		}
		if code < 0 || code >= 2048 {
			return nil, fmt.Errorf("F5 dictionary reference%d", code)
		}
		for links := 0; prefix[code] != -1; links++ {
			if links >= 2048 {
				return nil, fmt.Errorf("F5 cyclic dictionary")
			}
			stack = append(stack, suffix[code])
			code = int(prefix[code])
			if code < 0 || code >= 2048 {
				return nil, fmt.Errorf("F5 dictionary prefix outside2048")
			}
		}
		first = int(suffix[code])
		out = append(out, byte(first))
		prefix[next] = int16(previous)
		suffix[next] = byte(first)
		previous = original
		next++
		if next >= 1<<width {
			width++
			if width > maximum {
				width = 9
				next = 256
				previous = -1
				for i := range prefix {
					prefix[i] = -1
				}
			}
		}
		for len(stack) > 0 && len(out) < size {
			out = append(out, stack[len(stack)-1])
			stack = stack[:len(stack)-1]
		}
	}
	return out, nil
}

func cockpitMask(pixels []byte) (mask []byte, split, bottom int, err error) {
	if len(pixels) != 64000 {
		return nil, 0, 0, fmt.Errorf("cockpit mask requires64000indices")
	}
	words := []int{}
	si, cx, state := 0, 65535, 0
	delimiters := 0
	for count := 0; count < 200000; count++ {
		if si >= 64000 {
			return nil, 0, 0, fmt.Errorf("cockpit mask missing terminator")
		}
		v := pixels[si]
		si++
		cx = (cx + 1) & 65535
		if state == 0 {
			if si >= 64000 {
				break
			}
			if v == 13 {
				delimiters++
				cx = (cx + 1) & 65535
				words = append(words, cx, 0, 32768)
				cx = 65535
				if pixels[si] == 5 {
					continue
				}
				words = append(words, 0)
				state = 1
			} else if v != 5 {
				words = append(words, cx)
				cx = 0
				state = 1
			}
		} else {
			if si >= 64000 {
				words = append(words, cx)
				break
			}
			if v == 5 {
				words = append(words, cx)
				cx = 0
				state = 0
			}
		}
	}
	words = append(words, 32768)
	if len(words)*2 > 0x960 || delimiters != 1 {
		return nil, 0, 0, fmt.Errorf("unsupported cockpit mask control layout")
	}
	pos, source := 0, 0
	for words[pos] < 32768 {
		source += words[pos] + words[pos+1]
		pos += 2
	}
	split = source
	bottom = (split+1)/320 - 1
	mask = make([]byte, 64000)
	dst, phase := 0, 0
	for pos = 0; pos < len(words); {
		skip := words[pos]
		pos++
		if skip&32768 != 0 {
			phase++
			if phase == 2 {
				break
			}
			continue
		}
		if pos >= len(words) {
			return nil, 0, 0, fmt.Errorf("truncated mask run")
		}
		n := words[pos]
		pos++
		dst += skip
		if dst+n > 64000 {
			return nil, 0, 0, fmt.Errorf("cockpit mask run exceeds image")
		}
		for i := dst; i < dst+n; i++ {
			mask[i] = byte(phase + 1)
		}
		dst += n
	}
	return mask, split, bottom, nil
}

func grayIndexPNG(p []byte, w, h int) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, &image.Gray{Pix: p, Stride: w, Rect: image.Rect(0, 0, w, h)})
	return b.Bytes(), err
}
func contextPNG(p []byte, w, h int, rgb6 []byte) ([]byte, error) {
	pal := make(color.Palette, 256)
	if len(rgb6) != 768 {
		return nil, fmt.Errorf("invalid cockpit context palette")
	}
	for i := range pal {
		r, g, b := rgb6[3*i], rgb6[3*i+1], rgb6[3*i+2]
		pal[i] = color.RGBA{r<<2 | r>>4, g<<2 | g>>4, b<<2 | b>>4, 255}
	}
	var b bytes.Buffer
	err := png.Encode(&b, &image.Paletted{Pix: p, Stride: w, Rect: image.Rect(0, 0, w, h), Palette: pal})
	return b.Bytes(), err
}

func decodeCockpitVectors(raw []byte) ([]cockpit.Vector, int, error) {
	var result []cockpit.Vector
	at := 0
	for {
		if at >= len(raw) {
			return nil, 0, fmt.Errorf("vector has no zero terminator")
		}
		c := raw[at]
		at++
		if c == 0 {
			return result, at, nil
		}
		if at >= len(raw) {
			return nil, 0, fmt.Errorf("truncated vector count")
		}
		n := int(raw[at]) + 1
		at++
		if n > 128 || at+2*n > len(raw) {
			return nil, 0, fmt.Errorf("vector point bounds")
		}
		v := cockpit.Vector{Colour: c}
		for i := 0; i < n; i++ {
			v.Points = append(v.Points, [2]int16{int16(int8(raw[at])), int16(int8(raw[at+1]))})
			at += 2
		}
		result = append(result, v)
	}
}

func prepareWorldCockpit(out *OutputWriter, alloc *pathAllocator, m *Manifest, profile *AssetVersionProfile) error {
	l := profile.CockpitLayout
	if l == nil {
		return fmt.Errorf("profile has no supported cockpit layout")
	}
	loose := map[string]*Resource{}
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" {
			loose[strings.ToUpper(filepath.Base(r.Source.Path))] = r
		}
	}
	gs := loose[profile.Source(RoleSimulationExecutable).Name]
	if gs == nil || !profile.SourceMatches(RoleSimulationExecutable, gs.Source.PayloadSHA256) {
		return fmt.Errorf("cockpit preparation requires matching %s", profile.Source(RoleSimulationExecutable).Name)
	}
	doc := cockpitMetadata{Schema: cockpit.Schema, Profile: profile.Name, Files: map[string]Output{}, Data: cockpit.Assets{Schema: cockpit.Schema, Images: map[string]cockpit.Image{}, Definitions: map[string]cockpit.Definition{}, Atlases: map[string]cockpit.Image{}, Sprites: map[string]cockpit.Image{}, Literals: map[string]string{}, MapVectors: map[string][]cockpit.Vector{}, Opaque: map[string][]byte{}}, Notes: []string{"Selected VGA supplied-state recipes; no natural first-frame or simulation behavior asserted.", "Gray8 PNG bytes are palette indices. Runtime reads verified index payloads, never normal RGB-expanded PNGs.", "CPIT00 opaque/full05skip/tail copy differs from sprite00skip and world material operations.", "Driver6100 retained opaque; target-camera/contact/damage producers and native clock rates remain qualified."}}
	write := func(key, name, kind string, p []byte) error {
		o, e := out.Write("world/cockpit/"+name, p)
		if e != nil {
			return e
		}
		o.Kind = kind
		doc.Files[key] = o
		gs.Outputs = append(gs.Outputs, o)
		return nil
	}
	slice := func(s cockpitSourceSlice) ([]byte, error) {
		b, at, e := cockpitSourceBytes(gs.Payload, l, s.Section, s.CS, s.IP, s.Length)
		if e != nil {
			return nil, e
		}
		doc.Slices = append(doc.Slices, cockpitSliceProvenance{s.Name, s.Section, s.CS, s.IP, at, len(b), Hash(b)})
		if e = write("raw/"+s.Name, "raw/"+s.Name+".bin", "cockpit_source_slice", b); e != nil {
			return nil, e
		}
		return b, nil
	}
	nightPal, nightAt, err := cockpitSourceBytes(gs.Payload, l, l.NightPaletteSection, l.NightPaletteCS, l.NightPaletteIP, 768)
	if err != nil {
		return err
	}
	if err = write("night-palette", "night-fixed.rgb6", "cockpit_night_palette", nightPal); err != nil {
		return err
	}
	doc.Data.NightPalette = append([]byte(nil), nightPal...)
	doc.Slices = append(doc.Slices, cockpitSliceProvenance{"night-fixed-palette", l.NightPaletteSection, l.NightPaletteCS, l.NightPaletteIP, nightAt, len(nightPal), Hash(nightPal)})
	doc.Notes = append(doc.Notes, "VGA13e1 is the Alt+T alternative bank. GPS argument1 temporarily installs the current world staging bank10e1, not13e1. Map context PNGs use declared theatre DAY only; actual GPS preserves current world/night selector history on return.")
	day := []byte(nil)
	if r := loose["GULFDAY.PAL"]; r != nil && len(r.Payload) == 768 {
		day = append(day, r.Payload...)
		copy(day[39:42], day[45:48])
	}
	// Read only this profile's admitted carriers. An unrelated MAP.GS2 file is
	// retained by extraction, but must not make required cockpit readiness fail.
	f5names := []string{"APA_CPIT.PIC", "BLK_CPIT.PIC", "COB_CPIT.PIC", "COM_CPIT.PIC", "DEF_CPIT.PIC", "KIA_CPIT.PIC", "REPLAY.PIC"}
	if profile.WorldLayout == nil {
		return fmt.Errorf("cockpit profile has no world catalogue")
	}
	for _, name := range profile.WorldLayout.WorldNames {
		f5names = append(f5names, name+"MAP.GS2")
	}
	sort.Strings(f5names)
	for _, name := range f5names {
		if loose[name] == nil {
			return fmt.Errorf("required cockpit/map carrier %s missing", name)
		}
	}
	for _, name := range f5names {
		r := loose[name]
		pixels, e := DecodeF5(r.Payload, 64000)
		if e != nil {
			return fmt.Errorf("%s: %w", name, e)
		}
		img := cockpit.Image{Width: 320, Height: 200}
		if strings.HasSuffix(name, "_CPIT.PIC") {
			mask, split, bottom, e := cockpitMask(pixels)
			if e != nil {
				return fmt.Errorf("%s: %w", name, e)
			}
			img.Split, img.Bottom = split, bottom
			if e = write("mask/"+name, "images/"+name+".mask", "cockpit_full_restore_mask", mask); e != nil {
				return e
			}
		}
		if e = write("image/"+name, "images/"+name+".indices", "cockpit_indexed_pixels", pixels); e != nil {
			return e
		}
		png, e := grayIndexPNG(pixels, 320, 200)
		if e != nil {
			return e
		}
		if e = write("png/"+name, "images/"+name+".indices.png", "cockpit_gray8_index_png", png); e != nil {
			return e
		}
		pal := []byte(nil)
		label := ""
		if strings.HasSuffix(name, "_CPIT.PIC") {
			pal = day
			label = "GULFDAY-phase0-context"
		} else if len(name) >= 4 {
			// GPS uses the world staging bank. This is one explicit DAY
			// inspection context, not a universal map palette association.
			paletteName := name[:4] + "DAY.PAL"
			if r := loose[paletteName]; r != nil && len(r.Payload) == 768 {
				pal = append([]byte(nil), r.Payload...)
				copy(pal[39:42], pal[45:48])
				label = name[:4] + "DAY-phase0-context"
			}
		}
		if name != "REPLAY.PIC" && len(pal) == 768 {
			p, e := contextPNG(pixels, 320, 200, pal)
			if e != nil {
				return e
			}
			if e = write("context/"+name, "images/"+name+"."+label+".png", "cockpit_context_colour_png", p); e != nil {
				return e
			}
		}
		doc.Data.Images[name] = img
		doc.Sources = append(doc.Sources, r.Source)
	}
	for _, family := range []string{"APA", "COB", "DEF", "COM", "KIA", "BLK"} {
		r := loose[family+"_CPIT.DAT"]
		state := loose[family+"_STAT.DAT"]
		if r == nil || len(r.Payload) != 156 || state == nil || len(state.Payload) != 80 {
			return fmt.Errorf("missing %s CPIT/STAT definition", family)
		}
		d := cockpit.Definition{Raw: append([]byte(nil), r.Payload...), StateParameters: append([]byte(nil), state.Payload...)}
		for j := range d.Parameters {
			d.Parameters[j] = int16(binary.LittleEndian.Uint16(r.Payload[2*j:]))
		}
		d.SpanReplacement = binary.LittleEndian.Uint16(r.Payload[148:])
		copy(d.Colours[:], r.Payload[150:])
		doc.Data.Definitions[family] = d
		doc.Sources = append(doc.Sources, r.Source, state.Source)
	}
	if l.FontFile < 0 || l.FontFile+l.FontBytes > len(gs.Payload) || l.SineFile < 0 || l.SineFile+2*l.SineWords > len(gs.Payload) {
		return fmt.Errorf("HUD font/sine source bounds")
	}
	font := gs.Payload[l.FontFile : l.FontFile+l.FontBytes]
	if len(font) != 670 || !bytes.Equal(font[:5], []byte{2, 32, 126, 6, 6}) {
		return fmt.Errorf("HUD embedded font incompatible")
	}
	doc.Data.Font = cockpit.Font{First: 32, Last: 126, Width: 6, Height: 6, Advances: append([]byte(nil), font[5:100]...), Rows: append([]byte(nil), font[100:]...)}
	if err = write("font", "font/default.raw", "cockpit_default_font", font); err != nil {
		return err
	}
	fontPixels := make([]byte, 96*36)
	for i := 0; i < 95; i++ {
		x, y := (i%16)*6, (i/16)*6
		for row := 0; row < 6; row++ {
			for col := 0; col < 6; col++ {
				if font[100+6*i+row]&(0x80>>col) != 0 {
					fontPixels[(y+row)*96+x+col] = 1
				}
			}
		}
	}
	fontPNG, e := grayIndexPNG(fontPixels, 96, 36)
	if e != nil {
		return e
	}
	if err = write("fontmask", "font/default.mask.png", "hud_binary_glyph_mask", fontPNG); err != nil {
		return err
	}
	for i := 0; i < l.SineWords; i++ {
		doc.Data.Sine = append(doc.Data.Sine, int16(binary.LittleEndian.Uint16(gs.Payload[l.SineFile+2*i:])))
	}
	for i := range doc.Data.Needles {
		angle := uint16(i * 0x200)
		doc.Data.Needles[i] = [2]int16{int16(int32(doc.Data.Sin(angle)) * 10 >> 14), -int16(int32(doc.Data.Sin(angle+0x4000)) * 8 >> 14)}
	}
	var needleBytes bytes.Buffer
	for _, pair := range doc.Data.Needles {
		_ = binary.Write(&needleBytes, binary.LittleEndian, pair)
	}
	if err = write("needles", "needle-endpoints.i16le", "hud_generated_needle_pairs", needleBytes.Bytes()); err != nil {
		return err
	}
	for _, s := range l.Atlases {
		b, e := slice(s)
		if e != nil {
			return e
		}
		doc.Data.Atlases[s.Name] = cockpit.Image{Width: s.Width, Height: s.Height}
		if e = write("atlas/"+s.Name, "atlases/"+s.Name+".indices", "hud_indexed_atlas", b); e != nil {
			return e
		}
		p, e := grayIndexPNG(b, s.Width, s.Height)
		if e != nil {
			return e
		}
		if e = write("atlaspng/"+s.Name, "atlases/"+s.Name+".png", "hud_gray8_index_png", p); e != nil {
			return e
		}
	}
	for _, s := range l.Sprites {
		s.Length = 4 + s.Width*s.Height
		b, e := slice(s)
		if e != nil {
			return e
		}
		if int(binary.LittleEndian.Uint16(b)) != s.Width || int(binary.LittleEndian.Uint16(b[2:])) != s.Height {
			return fmt.Errorf("HUD sprite %s dimensions", s.Name)
		}
		doc.Data.Sprites[s.Name] = cockpit.Image{Width: s.Width, Height: s.Height}
		if e = write("sprite/"+s.Name, "sprites/"+s.Name+".indices", "hud_indexed_sprite", b[4:]); e != nil {
			return e
		}
		p, e := grayIndexPNG(b[4:], s.Width, s.Height)
		if e != nil {
			return e
		}
		if e = write("spritepng/"+s.Name, "sprites/"+s.Name+".png", "hud_gray8_index_png", p); e != nil {
			return e
		}
	}
	for _, s := range l.Texts {
		b, e := slice(s)
		if e != nil {
			return e
		}
		if b[len(b)-1] != 0 {
			return fmt.Errorf("HUD literal %s not terminated", s.Name)
		}
		doc.Data.Literals[s.Name] = string(b[:len(b)-1])
	}
	for _, line := range l.SceneInitialization {
		doc.Data.ScenePresentation.Initialization = append(doc.Data.ScenePresentation.Initialization, cockpit.PresentationLine{Text: doc.Data.Literals[line.Name], X: line.X, Y: line.Y, Colour: line.Colour, Style: line.Style})
	}
	prompt := func(p cockpitScenePromptLayout) cockpit.ScenePrompt {
		return cockpit.ScenePrompt{Text: doc.Data.Literals[p.Name], Foreground: p.Foreground, Background: p.Background, CenterX: p.CenterX, TextY: p.TextY, LeftPadding: p.LeftPadding, RightPadding: p.RightPadding, Top: p.Top, Bottom: p.Bottom, Style: p.Style, MeasureStyle: p.MeasureStyle}
	}
	doc.Data.ScenePresentation.Quit = prompt(l.SceneQuit)
	doc.Data.ScenePresentation.End = prompt(l.SceneEnd)
	for _, s := range l.Opaque {
		b, e := slice(s)
		if e != nil {
			return e
		}
		doc.Data.Opaque[s.Name] = append([]byte(nil), b...)
	}
	doc.Data.ScenePresentation.ConfirmationRemap = append([]byte(nil), doc.Data.Opaque["scene-confirmation-remap"]...)
	if e := doc.Data.ScenePresentation.Validate(); e != nil {
		return e
	}
	b, e := slice(cockpitSourceSlice{Name: "attitude-vector", Section: l.AttitudeSection, CS: l.AttitudeCS, IP: l.AttitudeIP, Length: l.AttitudeBytes})
	if e != nil {
		return e
	}
	doc.Data.Vectors, _, e = decodeCockpitVectors(b)
	if e != nil {
		return e
	}
	for _, s := range l.Vectors {
		b, at, e := cockpitSourceBytes(gs.Payload, l, s.Section, s.CS, s.IP, s.Length)
		if e != nil {
			return e
		}
		v, n, e := decodeCockpitVectors(b)
		if e != nil {
			return fmt.Errorf("HUD vector %s framing: %w", s.Name, e)
		}
		if n != len(b) {
			return fmt.Errorf("HUD vector %s uses%d of%d source bytes", s.Name, n, len(b))
		}
		doc.Data.MapVectors[s.Name] = v
		doc.Slices = append(doc.Slices, cockpitSliceProvenance{s.Name, s.Section, s.CS, s.IP, at, n, Hash(b)})
		if e = write("raw/"+s.Name, "raw/"+s.Name+".bin", "hud_vector_source", b); e != nil {
			return e
		}
	}
	vectorRoots := append([]int(nil), l.MapVectorRoots...)
	table, _, e := cockpitSourceBytes(gs.Payload, l, -1, l.MapVectorCS, l.MapVectorTable, 24)
	if e != nil {
		return e
	}
	for i := 0; i < 12; i++ {
		vectorRoots = append(vectorRoots, int(binary.LittleEndian.Uint16(table[2*i:])))
	}
	seen := map[int]bool{}
	for _, ip := range vectorRoots {
		if seen[ip] {
			continue
		}
		seen[ip] = true
		if ip < l.MapVectorStart || ip >= l.MapVectorEnd {
			return fmt.Errorf("map vector root outside carrier")
		}
		b, at, e := cockpitSourceBytes(gs.Payload, l, -1, l.MapVectorCS, ip, l.MapVectorEnd-ip)
		if e != nil {
			return e
		}
		v, n, e := decodeCockpitVectors(b)
		if e != nil {
			return e
		}
		name := fmt.Sprintf("map-%04x", ip)
		doc.Data.MapVectors[name] = v
		doc.Slices = append(doc.Slices, cockpitSliceProvenance{name, -1, l.MapVectorCS, ip, at, n, Hash(b[:n])})
		if e = write("raw/"+name, "raw/"+name+".bin", "hud_vector_source", b[:n]); e != nil {
			return e
		}
	}
	doc.Sources = append(doc.Sources, gs.Source)
	raw, e := JsonBytes(doc)
	if e != nil {
		return e
	}
	o, e := out.Write("world/cockpit/metadata.json", raw)
	if e != nil {
		return e
	}
	o.Kind = "cockpit_hud_metadata"
	gs.Outputs = append(gs.Outputs, o)
	m.CockpitMetadata = o.Path
	return nil
}

func loadCockpitAssets(reader *preparedReader, m Manifest) (*cockpit.Assets, error) {
	o, ok := reader.outputs[m.CockpitMetadata]
	if !ok || o.Kind != "cockpit_hud_metadata" {
		return nil, fmt.Errorf("required cockpit/HUD metadata missing; rebuild assets")
	}
	raw, e := reader.verified(o, 8<<20)
	if e != nil {
		return nil, e
	}
	var doc cockpitMetadata
	if e = json.Unmarshal(raw, &doc); e != nil {
		return nil, e
	}
	if doc.Schema != cockpit.Schema {
		return nil, fmt.Errorf("unsupported cockpit metadata schema")
	}
	if e := doc.Data.ScenePresentation.Validate(); e != nil {
		return nil, fmt.Errorf("required scene presentation missing: %w", e)
	}
	profile := &IslandIce469085
	if doc.Profile != profile.Name {
		return nil, fmt.Errorf("unsupported cockpit source profile %q", doc.Profile)
	}
	matchedSource := false
	for _, source := range doc.Sources {
		if strings.EqualFold(filepath.Base(source.Path), profile.Source(RoleSimulationExecutable).Name) && profile.SourceMatches(RoleSimulationExecutable, source.PayloadSHA256) {
			matchedSource = true
		}
	}
	if !matchedSource {
		return nil, fmt.Errorf("cockpit metadata lacks recognized simulation-source provenance; rebuild assets")
	}
	doc.Data.NightPalette, e = reader.verified(doc.Files["night-palette"], 768)
	if e != nil {
		return nil, fmt.Errorf("required selected-VGA night palette: %w", e)
	}
	for name, img := range doc.Data.Images {
		img.Pixels, e = reader.verified(doc.Files["image/"+name], 1<<20)
		if e != nil {
			return nil, e
		}
		if strings.HasSuffix(name, "_CPIT.PIC") {
			img.FullMask, e = reader.verified(doc.Files["mask/"+name], 1<<20)
			if e != nil {
				return nil, e
			}
		}
		doc.Data.Images[name] = img
	}
	for name, img := range doc.Data.Atlases {
		img.Pixels, e = reader.verified(doc.Files["atlas/"+name], 1<<20)
		if e != nil {
			return nil, e
		}
		doc.Data.Atlases[name] = img
	}
	for name, img := range doc.Data.Sprites {
		img.Pixels, e = reader.verified(doc.Files["sprite/"+name], 1<<20)
		if e != nil {
			return nil, e
		}
		doc.Data.Sprites[name] = img
	}
	// Preparation31 already retains this exact verified raw resource. Decode
	// the source presentation tables once; never open originals during play.
	if rawSource, ok := reader.outputs["world/raw/GS2.GS2"]; ok {
		raw, err := reader.verified(rawSource, 1<<20)
		if err != nil {
			return nil, err
		}
		if Hash(raw) == IslandIce469085.Source(RoleSimulationExecutable).SHA256 {
			if err = loadFlightFeedbackTables(raw, &doc.Data); err != nil {
				return nil, err
			}
		}
	}
	if e = doc.Data.Validate(); e != nil {
		return nil, e
	}
	if m.PreparationVersion >= 17 {
		for i := 1; i <= 20; i++ {
			if _, ok := doc.Data.Literals[fmt.Sprintf("notification-%02d", i)]; !ok {
				return nil, fmt.Errorf("prepared combat notification%d missing; rebuild assets", i)
			}
		}
	}
	if m.PreparationVersion >= 19 {
		for _, name := range []string{"notification-hq", "gps-landed", "gps-achieved", "gps-hold-label", "gps-hold-0", "gps-hold-1", "gps-hold-2", "gps-weapons-free", "gps-weapons-hold", "gps-altitude-speed", "gps-altitude-1", "gps-altitude-2", "gps-altitude-3", "gps-speed-1", "gps-speed-2", "gps-speed-3", "gps-speed-4"} {
			if text, ok := doc.Data.Literals[name]; !ok || text == "" {
				return nil, fmt.Errorf("prepared cockpit literal %s missing; rebuild assets", name)
			}
		}
	}
	if m.PreparationVersion >= 20 && doc.Data.Literals["gps-hold-3"] != "Occupied" {
		return nil, fmt.Errorf("prepared occupied hold literal missing; rebuild assets")
	}
	if m.PreparationVersion >= 21 {
		for name, size := range map[string]int{"gps-damage-body": 91 * 67, "gps-damage-yellow": 49, "gps-damage-red": 49} {
			if len(doc.Data.Sprites[name].Pixels) != size {
				return nil, fmt.Errorf("missing source GPS damage sprite %s", name)
			}
		}
		if len(doc.Data.Opaque["gps-damage-positions"]) != 40 {
			return nil, fmt.Errorf("missing source GPS damage positions")
		}
	}
	if m.PreparationVersion >= 23 && doc.Data.Literals["notification-22"] != ": S.O.S." {
		return nil, fmt.Errorf("prepared source SOS literal missing; rebuild assets")
	}
	if m.PreparationVersion >= 25 && doc.Data.Literals["notification-21"] != ": Thanks!" {
		return nil, fmt.Errorf("prepared source rescued-actor literal missing; prepare25")
	}
	if m.PreparationVersion >= 30 && (doc.Data.Literals["gps-support-aircraft"] != "- Aircraft" || doc.Data.Literals["gps-support-artillery"] != "- Artillery : 3") {
		return nil, fmt.Errorf("prepared source Mission Assets captions missing; prepare30")
	}
	if len(doc.Data.MapVectors["gps-fly-cursor"]) != 2 || len(doc.Data.Sprites["gps-overview-box"].Pixels) != 11*9 {
		return nil, fmt.Errorf("GPS cursor/overview carriers missing; rebuild assets")
	}
	return &doc.Data, nil
}
