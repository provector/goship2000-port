package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	cockpitdata "gunship2000-assets/game/cockpit"
	worlddata "gunship2000-assets/game/world"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// OfficeAssets owns immutable prepared data. Scene composition always copies a
// picture before changing it; native scratch-bank aliasing is not a disk asset.
type OfficeAssets struct {
	PreparationVersion int `json:"-"`
	Root               string
	Metadata           OfficeMetadata
	Images             map[string][]byte
	Palette            color.Palette
	Fonts              map[int]*BitmapFont
	Menus              *MenuAssets
	Startup            *StartupAssets
	Missions           *MissionAssets
	Audio              *AudioAssets
	World              *worlddata.Bundle
	Cockpit            *cockpitdata.Assets
	Debrief            *DebriefAssets
	Replay             *ReplayAssets
	Editor             *EditorAssets
}

type preparedReader struct {
	historical *historicalProjection
	packs      *packedStore
	observed   map[string]bool
	canonical  map[string]leafProof
	root       string
	outputs    map[string]Output
}

func newPreparedReader(root string) (*preparedReader, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("prepared asset directory: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("prepared asset path is not a directory: %s", root)
	}
	packs, err := openPacks(resolved)
	if err != nil {
		return nil, err
	}
	return &preparedReader{root: resolved, outputs: map[string]Output{}, packs: packs}, nil
}

func (r *preparedReader) read(relative string, limit int64) ([]byte, error) {
	// Prepared names are slash-relative even on Windows. Reject traversal rather
	// than cleaning it, and resolve symlinks before accepting the final path.
	if relative == "" || strings.Contains(relative, "\\") || path.IsAbs(relative) || path.Clean(relative) != relative || strings.Contains(relative, ":") {
		return nil, fmt.Errorf("unsafe prepared asset path %q", relative)
	}
	for _, component := range strings.Split(relative, "/") {
		if component == ".." || component == "." || component == "" {
			return nil, fmt.Errorf("unsafe prepared asset path %q", relative)
		}
	}
	if r.observed != nil {
		r.observed[relative] = true
	}
	if r.packs != nil && relative != "manifest.json" {
		return r.packs.read(relative, limit)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(r.root, filepath.FromSlash(relative)))
	if err != nil {
		return nil, fmt.Errorf("prepared asset %q: %w", relative, err)
	}
	rel, err := filepath.Rel(r.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("prepared asset %q escapes directory", relative)
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("prepared asset %q is not a regular file within %d-byte limit", relative, limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("prepared asset %q exceeds limit", relative)
	}
	return data, nil
}

func (r *preparedReader) verified(out Output, limit int64) ([]byte, error) {
	canonical, ok := r.outputs[out.Path]
	if !ok || canonical.Size != out.Size || canonical.SHA256 != out.SHA256 || canonical.Kind != out.Kind {
		return nil, fmt.Errorf("output %q does not match prepared manifest", out.Path)
	}
	data, err := r.read(out.Path, limit)
	if err != nil {
		return nil, err
	}
	if len(data) != out.Size || Hash(data) != out.SHA256 {
		return nil, fmt.Errorf("prepared asset %q size/SHA-256 mismatch", out.Path)
	}
	return data, nil
}

func preparedOutput(outputs []Output, kind string) (Output, error) {
	var result Output
	found := false
	for _, out := range outputs {
		if out.Kind == kind {
			if found {
				return result, fmt.Errorf("ambiguous %s outputs", kind)
			}
			result = out
			found = true
		}
	}
	if !found {
		return result, fmt.Errorf("missing %s output", kind)
	}
	return result, nil
}

func LoadOfficeAssets(directory string) (*OfficeAssets, error) {
	reader, err := newPreparedReader(directory)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.loadOffice()
}
func (reader *preparedReader) loadOffice() (*OfficeAssets, error) {
	raw, err := reader.read("manifest.json", 64<<20)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("prepared manifest: %w", err)
	}
	if !compatiblePreparation(manifest) || manifest.OfficeMetadata == "" {
		return nil, fmt.Errorf("office requires a compatible preparation version, manifest schema1 and office_metadata; rebuild with a supported office profile")
	}
	for _, resource := range manifest.Resources {
		if resource == nil {
			return nil, fmt.Errorf("prepared manifest contains a null resource")
		}
		for _, out := range resource.Outputs {
			if previous, ok := reader.outputs[out.Path]; ok && previous != out {
				return nil, fmt.Errorf("conflicting manifest output %q", out.Path)
			}
			reader.outputs[out.Path] = out
		}
	}
	metaOutput, ok := reader.outputs[manifest.OfficeMetadata]
	if !ok || metaOutput.Kind != "office_presentation_metadata" {
		return nil, fmt.Errorf("office metadata is not an indexed generated output")
	}
	raw, err = reader.verified(metaOutput, 4<<20)
	if err != nil {
		return nil, err
	}
	assets := &OfficeAssets{PreparationVersion: manifest.PreparationVersion, Root: reader.root, Images: map[string][]byte{}, Fonts: map[int]*BitmapFont{}}
	if err = json.Unmarshal(raw, &assets.Metadata); err != nil {
		return nil, fmt.Errorf("office metadata: %w", err)
	}
	if err = validateOfficeMetadata(assets.Metadata); err != nil {
		return nil, err
	}
	required := map[string]bool{"BASE.PIC": false, "BASE_AN.PIC": false, "BASE_AN1.PIC": false, "BASE_AN2.PIC": false, "POINTER.PIC": false}
	paletteSource := ""
	palettePayloadHash := ""
	for _, asset := range assets.Metadata.Assets {
		if _, needed := required[asset.Name]; !needed {
			continue
		}
		if required[asset.Name] {
			return nil, fmt.Errorf("duplicate office asset %q", asset.Name)
		}
		out, e := preparedOutput(asset.Outputs, "decoded_indices")
		if e != nil {
			return nil, fmt.Errorf("%s: %w", asset.Name, e)
		}
		pixels, e := reader.verified(out, 64000)
		if e != nil {
			return nil, e
		}
		if len(pixels) != 320*200 || Hash(pixels) != asset.PixelSHA256 {
			return nil, fmt.Errorf("%s requires 320x200 matching indices", asset.Name)
		}
		if asset.Palette.Diagnostic || asset.Palette.Source == "" {
			return nil, fmt.Errorf("%s has no supported office palette association", asset.Name)
		}
		if paletteSource != "" && asset.Palette.Source != paletteSource {
			return nil, fmt.Errorf("office pictures name inconsistent palettes")
		}
		paletteSource = asset.Palette.Source
		if palettePayloadHash != "" && asset.Palette.SHA256 != palettePayloadHash {
			return nil, fmt.Errorf("office pictures name inconsistent palette source identities")
		}
		palettePayloadHash = asset.Palette.SHA256
		assets.Images[asset.Name] = pixels
		required[asset.Name] = true
	}
	for name, present := range required {
		if !present {
			return nil, fmt.Errorf("required office picture %s is missing", name)
		}
	}
	var paletteResource *Resource
	for _, resource := range manifest.Resources {
		if resource.ExportPath == paletteSource {
			if paletteResource != nil {
				return nil, fmt.Errorf("ambiguous office palette %s", paletteSource)
			}
			paletteResource = resource
		}
	}
	if paletteResource == nil {
		return nil, fmt.Errorf("office palette %q is missing", paletteSource)
	}
	if paletteResource.Source.PayloadSHA256 != palettePayloadHash {
		return nil, fmt.Errorf("office palette source identity differs from picture association")
	}
	paletteOutput, err := preparedOutput(paletteResource.Outputs, "exact_rgb6_components")
	if err != nil {
		return nil, err
	}
	components, err := reader.verified(paletteOutput, 768)
	if err != nil {
		return nil, err
	}
	if len(components) != 768 {
		return nil, fmt.Errorf("office palette must establish all 256 RGB6 entries")
	}
	assets.Palette = make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		rgb := components[i*3 : i*3+3]
		for _, component := range rgb {
			if component > 63 {
				return nil, fmt.Errorf("office palette component exceeds RGB6")
			}
		}
		assets.Palette[i] = color.NRGBA{rgb[0]<<2 | rgb[0]>>4, rgb[1]<<2 | rgb[1]>>4, rgb[2]<<2 | rgb[2]>>4, 255}
	}
	for _, fontAsset := range assets.Metadata.Fonts {
		for _, out := range fontAsset.Outputs {
			if out.Kind != "bitmap_font_metrics" {
				continue
			}
			data, e := reader.verified(out, 1<<20)
			if e != nil {
				return nil, e
			}
			var metrics FontMetrics
			if e = json.Unmarshal(data, &metrics); e != nil {
				return nil, fmt.Errorf("font metrics %s: %w", out.Path, e)
			}
			if metrics.Selector != 3 && metrics.Selector != 5 {
				continue
			}
			if assets.Fonts[metrics.Selector] != nil {
				return nil, fmt.Errorf("duplicate font selector %d", metrics.Selector)
			}
			maskOutput, ok := reader.outputs[metrics.Atlas]
			if !ok || maskOutput.Kind != "bitmap_font_mask_atlas" {
				return nil, fmt.Errorf("font%d mask is missing", metrics.Selector)
			}
			data, e = reader.verified(maskOutput, 1<<20)
			if e != nil {
				return nil, e
			}
			cfg, e := png.DecodeConfig(bytes.NewReader(data))
			if e != nil {
				return nil, e
			}
			if cfg.Width != metrics.AtlasSize[0] || cfg.Height != metrics.AtlasSize[1] || cfg.Width*cfg.Height > 1<<20 {
				return nil, fmt.Errorf("font%d atlas dimensions differ from metrics", metrics.Selector)
			}
			decoded, e := png.Decode(bytes.NewReader(data))
			if e != nil {
				return nil, e
			}
			mask, ok := decoded.(*image.Paletted)
			if !ok {
				return nil, fmt.Errorf("font%d atlas is not indexed", metrics.Selector)
			}
			font, e := NewBitmapFont(metrics, mask)
			if e != nil {
				return nil, e
			}
			assets.Fonts[metrics.Selector] = font
		}
	}
	for _, selector := range []int{3, 5} {
		if assets.Fonts[selector] == nil {
			return nil, fmt.Errorf("office font%d is missing", selector)
		}
	}
	return assets, nil
}

func validateOfficeMetadata(m OfficeMetadata) error {
	if m.Schema != officeMetadataSchema || m.OuterType != "FORM/SCRN" {
		return fmt.Errorf("unsupported office metadata schema/type")
	}
	if len(m.Components) != 105 || len(m.Groups) != 32 || len(m.Areas) != 10 || len(m.Animations) != 5 || len(m.Lori) != 3 || len(m.Ranks) != 12 || len(m.Missions) != 5 || len(m.Theatres) != 4 || len(m.Denials) != 3 || len(m.Captions) != 9 {
		return fmt.Errorf("office metadata has incomplete presentation tables")
	}
	for i, c := range m.Components {
		if c.Index != i || !preparedOfficeRectFits(c.Source) || !preparedOfficeRectFits([4]int{c.Additive[0], c.Additive[1], c.Source[2], c.Source[3]}) {
			return fmt.Errorf("office component%d is outside source/destination bounds", i)
		}
		if c.SourceAsset != "BASE_AN.PIC" && c.SourceAsset != "BASE_AN1.PIC" && c.SourceAsset != "BASE_AN2.PIC" {
			return fmt.Errorf("office component%d has unknown source asset", i)
		}
	}
	seenComponents := make([]bool, len(m.Components))
	for ordinal, g := range m.Groups {
		if g.Key != ordinal || len(g.Records) == 0 {
			return fmt.Errorf("office group index%d has unsupported key/empty records", ordinal)
		}
		for _, index := range g.Records {
			if index < 0 || index >= len(m.Components) || m.Components[index].GroupKey != g.Key || seenComponents[index] {
				return fmt.Errorf("office group%d references invalid component%d", g.Key, index)
			}
			seenComponents[index] = true
		}
	}
	for index, seen := range seenComponents {
		if !seen {
			return fmt.Errorf("office component%d is absent from groups", index)
		}
	}
	for ordinal, a := range m.Animations {
		if a.Key != ordinal {
			return fmt.Errorf("office animation index%d has unexpected key", ordinal)
		}
		if (a.Key == 1 || a.Key == 2) && len(a.Frames) != a.FrameCount {
			return fmt.Errorf("office direct animation%d has missing frames", a.Key)
		}
		if a.FrameCount <= 0 || a.FrameCount > 255 || a.HoldCount <= 0 || a.HoldCount > 255 || a.Frame < 0 || a.Frame >= a.FrameCount || a.Hold < 0 || a.Hold >= a.HoldCount || a.Pause < 0 || a.Pause > 1 {
			return fmt.Errorf("office animation%d has invalid count", a.Key)
		}
		if !preparedOfficeRectFits([4]int{a.Destination[0], a.Destination[1], a.Size[0], a.Size[1]}) {
			return fmt.Errorf("office animation%d destination/size outside bounds", a.Key)
		}
		if a.Key == 1 || a.Key == 2 {
			if a.Divisor <= 0 || a.Asset != "BASE_AN.PIC" {
				return fmt.Errorf("office direct animation%d has unsupported source/divisor", a.Key)
			}
		}
		if a.Key == 4 {
			if a.Asset != "BASE_AN2.PIC" || len(a.CallbackSources) != 2 {
				return fmt.Errorf("office redraw callback has incomplete source metadata")
			}
			for _, parity := range []string{"even", "odd"} {
				rectangle, ok := a.CallbackSources[parity]
				if !ok || !preparedOfficeRectFits(rectangle) || rectangle[2] != a.Size[0] || rectangle[3] != a.Size[1] {
					return fmt.Errorf("office redraw callback has invalid %s source", parity)
				}
			}
		}
		for ordinal, frame := range a.Frames {
			if frame.Index != ordinal || frame.Destination != a.Destination || frame.Source[2] != a.Size[0] || frame.Source[3] != a.Size[1] {
				return fmt.Errorf("office animation%d frame%d inconsistent geometry/index", a.Key, ordinal)
			}
			if !preparedOfficeRectFits(frame.Source) || !preparedOfficeRectFits([4]int{frame.Destination[0], frame.Destination[1], frame.Source[2], frame.Source[3]}) {
				return fmt.Errorf("office animation%d frame%d outside bounds", a.Key, frame.Index)
			}
		}
	}
	seenAreas := map[int]bool{}
	for _, area := range m.Areas {
		if area.Key < 0 || area.Key > 9 || seenAreas[area.Key] {
			return fmt.Errorf("office AREA key%d is unsupported or duplicated", area.Key)
		}
		seenAreas[area.Key] = true
		if !preparedOfficeRectFits(area.Rectangle) {
			return fmt.Errorf("office AREA%d outside scene bounds", area.Key)
		}
	}
	for ordinal, lori := range m.Lori {
		if lori.Key != ordinal || lori.Mode < 0 || lori.Mode > 1 || lori.SourceAsset != "BASE_AN.PIC" {
			return fmt.Errorf("office LORI%d has unsupported key/mode/source", ordinal)
		}
		if lori.Divisor <= 0 || !preparedOfficeRectFits([4]int{lori.Base[0], lori.Base[1], lori.Cell[0], lori.Cell[1]}) {
			return fmt.Errorf("office LORI%d has unsupported geometry", lori.Key)
		}
	}
	for i, theatre := range m.Theatres {
		if theatre.Selector != i || theatre.Colour < 0 || theatre.Colour > 255 || len(theatre.Segments) < 3 || len(theatre.Segments) > 4 {
			return fmt.Errorf("office theatre%d has unsupported selector/segments/colour", i)
		}
		for _, segment := range theatre.Segments {
			for axis := 0; axis < 4; axis++ {
				limit := 320
				if axis%2 == 1 {
					limit = 200
				}
				if segment[axis] < 0 || segment[axis] >= limit {
					return fmt.Errorf("office theatre%d line endpoint outside bounds", i)
				}
			}
		}
	}
	for _, labels := range [][]OfficeTextSource{m.Ranks, m.Missions} {
		for i, label := range labels {
			if label.Selector != i || len(label.Text) > 1024 {
				return fmt.Errorf("office text table has unsupported selector/length")
			}
		}
	}
	for i, key := range []int{0, 1, 2, 3, 4, 5, 8, 9, 10} {
		if m.Captions[i].Selector != key || len(m.Captions[i].Text) > 1024 {
			return fmt.Errorf("office caption table has unsupported key/length")
		}
	}
	for _, denial := range m.Denials {
		if len(denial.Text) > 1024 {
			return fmt.Errorf("office denial caption exceeds supported length")
		}
	}
	if len(m.Initialization) == 0 || len(m.Rules) == 0 {
		return fmt.Errorf("office initialization/rules are missing")
	}
	return nil
}

// Subtraction avoids overflow when user-supplied metadata contains huge sizes.
func preparedOfficeRectFits(r [4]int) bool {
	return r[0] >= 0 && r[1] >= 0 && r[2] > 0 && r[3] > 0 && r[2] <= 320 && r[3] <= 200 && r[0] <= 320-r[2] && r[1] <= 200-r[3]
}
