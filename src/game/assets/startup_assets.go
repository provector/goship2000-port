package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strings"
)

const startupMetadataSchema = 1

const maxPreparedStartupFrames = 2048

type startupFrameMetadata struct {
	Phase              string  `json:"phase"`
	DelayBeforeSeconds float64 `json:"delay_before_seconds"`
	HoldSeconds        float64 `json:"hold_seconds"`
	SourceStart        int     `json:"source_start"`
	SourceEnd          int     `json:"source_end"`
	HoldRaw            uint16  `json:"hold_raw"`
	PaletteIndex       int     `json:"palette_index"`
	PixelSHA256        string  `json:"pixel_sha256"`
}

type startupStageMetadata struct {
	Schema             int                    `json:"schema_version"`
	Name               string                 `json:"name"`
	Source             SourceLocation         `json:"source"`
	Player             SourceLocation         `json:"player"`
	Width              int                    `json:"width"`
	Height             int                    `json:"height"`
	TimingBasis        string                 `json:"timing_basis"`
	CompressedConsumed int                    `json:"compressed_consumed"`
	LogicalBytes       int                    `json:"logical_bytes"`
	PaletteKnown       []bool                 `json:"palette_known"`
	Pixels             Output                 `json:"frame_indices"`
	Palettes           Output                 `json:"rgb6_palettes"`
	Frames             []startupFrameMetadata `json:"frames"`
}

type startupSequenceMetadata struct {
	Schema int      `json:"schema_version"`
	Stages []Output `json:"stages"`
	Notes  []string `json:"limits"`
}

type StartupAssets struct{ Stages []*StartupAnimation }

func exportStartupMetadata(out *OutputWriter, alloc *pathAllocator, m *Manifest) error {
	sources := map[SourceRole]*Resource{}
	for _, r := range m.Resources {
		for _, role := range []SourceRole{roleStartupLogo, roleStartupIntro, RoleLogoPlayer, RoleIntroPlayer} {
			if IslandIce469085.SourceMatches(role, r.Source.PayloadSHA256) {
				sources[role] = r
			}
		}
	}
	// This preparation profile includes player-owned palette/scheduling rules.
	// A similar extension or header never silently enables those interpretations.
	if len(sources) != 4 {
		var missing []string
		for _, role := range []SourceRole{roleStartupLogo, roleStartupIntro, RoleLogoPlayer, RoleIntroPlayer} {
			if sources[role] == nil {
				missing = append(missing, IslandIce469085.Source(role).Name)
			}
		}
		m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, UnsupportedVersionOperation{"original startup sequence", IslandIce469085.Name, "Missing or unsupported source identities: " + strings.Join(missing, ", ")})
		return nil
	}
	sequence := startupSequenceMetadata{Schema: startupMetadataSchema, Notes: []string{
		"LABS owns the SS logo; PLAYER owns GST.PAN, including its embedded credits. No captured pixels or runtime dictionary seed are decoder inputs.",
		"RGB6->RGB8 uses (v<<2)|(v>>4). SS unspecified unused palette entries are zero placeholders, not claims of installed black.",
		"Elapsed-time scheduling replaces native waits; source holds/order are retained. Timing/fade limitations are specified per stage.",
	}}
	for _, item := range []struct {
		role, player SourceRole
		decode       func([]byte) (*StartupAnimation, error)
	}{
		{roleStartupLogo, RoleLogoPlayer, DecodeStartupSS}, {roleStartupIntro, RoleIntroPlayer, DecodeStartupPAN},
	} {
		r := sources[item.role]
		animation, err := item.decode(r.Payload)
		if err != nil {
			return fmt.Errorf("startup %s: %w", r.name, err)
		}
		if len(animation.Frames) == 0 || len(animation.Frames)+len(animation.Prelude) > maxPreparedStartupFrames {
			return fmt.Errorf("startup frame count outside preparation limit")
		}
		doc := startupStageMetadata{Schema: startupMetadataSchema, Name: animation.Name, Source: r.Source, Player: sources[item.player].Source, Width: animation.Width, Height: animation.Height, TimingBasis: animation.TimingBasis, CompressedConsumed: animation.CompressedConsumed, LogicalBytes: animation.LogicalBytes, PaletteKnown: animation.PaletteKnown}
		var pixels, palettes []byte
		for fi, f := range append(append([]StartupFrame(nil), animation.Prelude...), animation.Frames...) {
			if len(f.Pixels) != StartupPixels || len(f.PaletteRGB6) != 768 {
				return fmt.Errorf("startup frame has invalid dimensions/palette")
			}
			paletteIndex := -1
			for i := 0; i < len(palettes); i += 768 {
				if bytes.Equal(palettes[i:i+768], f.PaletteRGB6) {
					paletteIndex = i / 768
					break
				}
			}
			if paletteIndex < 0 {
				paletteIndex = len(palettes) / 768
				palettes = append(palettes, f.PaletteRGB6...)
			}
			phase := "frame"
			if fi < len(animation.Prelude) {
				phase = "palette_fade"
			}
			doc.Frames = append(doc.Frames, startupFrameMetadata{phase, f.DelayBeforeSeconds, f.HoldSeconds, f.SourceStart, f.SourceEnd, f.HoldRaw, paletteIndex, Hash(f.Pixels)})
			pixels = append(pixels, f.Pixels...)
		}
		for _, g := range []struct {
			suffix, kind string
			raw          []byte
		}{{".startup.indices", "startup_frame_indices", pixels}, {".startup.rgb6", "startup_rgb6_palettes", palettes}} {
			if err = addGenerated(out, r, alloc, g.suffix, g.kind, g.raw); err != nil {
				return err
			}
			if g.kind == "startup_frame_indices" {
				doc.Pixels = r.Outputs[len(r.Outputs)-1]
			} else {
				doc.Palettes = r.Outputs[len(r.Outputs)-1]
			}
		}
		raw, err := JsonBytes(doc)
		if err != nil {
			return err
		}
		if err = addGenerated(out, r, alloc, ".startup.json", "startup_stage_metadata", raw); err != nil {
			return err
		}
		sequence.Stages = append(sequence.Stages, r.Outputs[len(r.Outputs)-1])
		if r.Conversion.Status == "unsupported" {
			m.Summary.Unsupported--
			m.Summary.UnsupportedFormats[r.Format]--
			if m.Summary.UnsupportedFormats[r.Format] == 0 {
				delete(m.Summary.UnsupportedFormats, r.Format)
			}
		}
		r.Conversion = ConversionResult{Status: "converted_startup_animation", Width: animation.Width, Height: animation.Height, RuntimeValidation: "Source-backed decoder; SS used palette range and PAN representative full frames agree with prior independent Step18 witnesses. All other frames are offline decoded, not newly runtime captured.", Palette: PaletteAssociation{Source: r.ExportPath, Evidence: "Palette embedded in actual SS/PAN source; SS logical indices are remapped by the source LABS loader", Confidence: "source_and_prior_runtime_witnesses"}}
		m.Summary.StartupAnimations++
		m.Summary.StartupFrames += len(animation.Frames) + len(animation.Prelude)
	}
	raw, err := JsonBytes(sequence)
	if err != nil {
		return err
	}
	logo := sources[roleStartupLogo]
	if err = addGenerated(out, logo, alloc, ".sequence.json", "startup_sequence_metadata", raw); err != nil {
		return err
	}
	m.StartupMetadata = logo.Outputs[len(logo.Outputs)-1].Path
	return nil
}

func LoadStartupAssets(directory string) (*StartupAssets, error) {
	reader, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.loadStartup()
}
func (reader *preparedReader) loadStartup() (*StartupAssets, error) {
	raw, err := reader.read("manifest.json", 64<<20)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if !compatiblePreparation(manifest) || manifest.StartupMetadata == "" {
		for _, unsupported := range manifest.UnsupportedVersionOperations {
			if unsupported.Operation == "original startup sequence" {
				return nil, fmt.Errorf("startup preparation unsupported: %s", unsupported.Reason)
			}
		}
		return nil, fmt.Errorf("prepared assets lack original startup animation outputs; rebuild/upgrade for C.1")
	}
	for _, r := range manifest.Resources {
		if r == nil {
			return nil, fmt.Errorf("null startup manifest resource")
		}
		for _, o := range r.Outputs {
			if old, ok := reader.outputs[o.Path]; ok && old != o {
				return nil, fmt.Errorf("conflicting startup output %s", o.Path)
			}
			reader.outputs[o.Path] = o
		}
	}
	metadata, ok := reader.outputs[manifest.StartupMetadata]
	if !ok || metadata.Kind != "startup_sequence_metadata" {
		return nil, fmt.Errorf("startup sequence not in manifest")
	}
	raw, err = reader.verified(metadata, 1<<20)
	if err != nil {
		return nil, err
	}
	var sequence startupSequenceMetadata
	if err = DecodeObject(raw, &sequence); err != nil {
		return nil, err
	}
	if sequence.Schema != startupMetadataSchema || len(sequence.Stages) != 2 {
		return nil, fmt.Errorf("unsupported startup sequence schema/stages")
	}
	assets := &StartupAssets{}
	for index, entry := range sequence.Stages {
		if entry.Kind != "startup_stage_metadata" {
			return nil, fmt.Errorf("invalid startup stage output")
		}
		raw, err = reader.verified(entry, 4<<20)
		if err != nil {
			return nil, err
		}
		var doc startupStageMetadata
		if err = DecodeObject(raw, &doc); err != nil {
			return nil, err
		}
		source, player := roleStartupLogo, RoleLogoPlayer
		if index == 1 {
			source, player = roleStartupIntro, RoleIntroPlayer
		}
		if doc.Schema != startupMetadataSchema || doc.Width != 320 || doc.Height != 200 || len(doc.Frames) < 1 || len(doc.Frames) > maxPreparedStartupFrames || len(doc.PaletteKnown) != 256 || !IslandIce469085.SourceMatches(source, doc.Source.PayloadSHA256) || !IslandIce469085.SourceMatches(player, doc.Player.PayloadSHA256) {
			return nil, fmt.Errorf("unsupported startup stage profile/geometry")
		}
		if doc.CompressedConsumed < 0 || uint64(doc.CompressedConsumed) > doc.Source.Length || doc.LogicalBytes < 1 || doc.LogicalBytes > maxStartupLogical {
			return nil, fmt.Errorf("invalid startup source/logical extents")
		}
		if doc.Pixels.Kind != "startup_frame_indices" || doc.Pixels.Size != len(doc.Frames)*StartupPixels || doc.Palettes.Kind != "startup_rgb6_palettes" || doc.Palettes.Size < 768 || doc.Palettes.Size%768 != 0 || doc.Palettes.Size > 768*maxPreparedStartupFrames {
			return nil, fmt.Errorf("invalid startup pack sizes")
		}
		var pixels []byte
		var media *packedFile
		if reader.packs != nil {
			media = reader.packs.catalog.Files[doc.Pixels.Path]
			if media == nil || media.Codec != "frames" || media.Size != doc.Pixels.Size || reader.outputs[doc.Pixels.Path] != doc.Pixels {
				return nil, fmt.Errorf("invalid packed intro indices")
			}
		} else {
			pixels, err = reader.verified(doc.Pixels, int64(maxPreparedStartupFrames*StartupPixels))
			if err != nil {
				return nil, err
			}
		}
		palettes, err := reader.verified(doc.Palettes, int64(768*maxPreparedStartupFrames))
		if err != nil {
			return nil, err
		}
		for _, v := range palettes {
			if v > 63 {
				return nil, fmt.Errorf("startup RGB6 component exceeds63")
			}
		}
		animation := &StartupAnimation{Name: doc.Name, Width: 320, Height: 200, SourceSHA256: doc.Source.PayloadSHA256, CompressedConsumed: doc.CompressedConsumed, LogicalBytes: doc.LogicalBytes, TimingBasis: doc.TimingBasis, PaletteKnown: doc.PaletteKnown}
		for i, f := range doc.Frames {
			if math.IsNaN(f.DelayBeforeSeconds) || math.IsInf(f.DelayBeforeSeconds, 0) || f.DelayBeforeSeconds < 0 || f.DelayBeforeSeconds > 60 || math.IsNaN(f.HoldSeconds) || math.IsInf(f.HoldSeconds, 0) || f.HoldSeconds < 0 || f.HoldSeconds > 60 || f.PaletteIndex < 0 || f.PaletteIndex >= len(palettes)/768 || f.SourceStart < 0 || f.SourceEnd < f.SourceStart || f.SourceEnd > doc.LogicalBytes {
				return nil, fmt.Errorf("invalid startup frame%d timing/range/palette", i)
			}
			var p []byte
			if media == nil {
				p = pixels[i*StartupPixels : (i+1)*StartupPixels]
				if Hash(p) != f.PixelSHA256 {
					return nil, fmt.Errorf("startup frame%d pixel hash mismatch", i)
				}
				for _, v := range p {
					if !doc.PaletteKnown[v] {
						return nil, fmt.Errorf("startup frame%d uses unspecified palette entry%d", i, v)
					}
				}
			}
			animation.Frames = append(animation.Frames, StartupFrame{digest: f.PixelSHA256, Pixels: p, PaletteRGB6: palettes[f.PaletteIndex*768 : (f.PaletteIndex+1)*768], DelayBeforeSeconds: f.DelayBeforeSeconds, HoldSeconds: f.HoldSeconds, SourceStart: f.SourceStart, SourceEnd: f.SourceEnd, HoldRaw: f.HoldRaw})
		}
		if media != nil {
			animation.media = media
			animation.packPath = filepath.Join(reader.root, preparedPacks[media.Pack])
		}
		assets.Stages = append(assets.Stages, animation)
	}
	return assets, nil
}

func StartupPalette(rgb6 []byte) color.Palette {
	result := make(color.Palette, 256)
	for i := range result {
		v := rgb6[i*3 : i*3+3]
		result[i] = color.NRGBA{v[0]<<2 | v[0]>>4, v[1]<<2 | v[1]>>4, v[2]<<2 | v[2]>>4, 255}
	}
	return result
}
