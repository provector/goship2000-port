package menus

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func WriteOfficeFrame(filename string, o *Office, pointer bool, seed uint64, rate float64) error {
	indices := o.Frame(pointer)
	report := struct {
		State           gamestate.OfficeState `json:"state"`
		Seed            uint64                `json:"seed"`
		Ticks           uint64                `json:"ticks"`
		NominalTickRate float64               `json:"nominal_tick_rate"`
		Pointer         bool                  `json:"pointer_included"`
		PointerXY       [2]int                `json:"pointer_xy"`
		HoverKey        int                   `json:"hover_key"`
		PixelSHA256     string                `json:"pixel_sha256"`
		Dimensions      [2]int                `json:"dimensions"`
		Timing          string                `json:"timing_scope"`
	}{o.State, seed, o.TickCount, rate, pointer, [2]int{o.PointerX, o.PointerY}, o.HoverKey, assetdata.Hash(indices), [2]int{320, 200}, "Logical tick cadence is a port approximation; local RNG has no native seed/history claim."}
	return writeIndexedFrame(filename, indices, o.Assets.Palette, o.Assets.Root, report)
}

func WriteFrontEndFrame(filename string, c *FrontEnd, pointer bool) error {
	if c.usesLocalQuitUIPalette() {
		return fmt.Errorf("quit popup uses a separate UI palette; capture its RGBA native presentation rather than publishing a misleading indexed preview")
	}
	indices := c.Frame(pointer)
	w, h, _ := indexedFrameSize(indices)
	report := map[string]any{"screen": c.Screen, "highlight": c.Highlight, "state": c.Store.State().OfficeState(), "save_revision": c.Store.State().Revision, "pixel_sha256": assetdata.Hash(indices), "dimensions": [2]int{w, h}, "scope": "Development preview; scratch JSON save only; no original-game observation"}
	report["identity"] = developmentIdentity(c.Office.Assets.Root)
	if c.DevelopmentScenario != nil {
		report["development_scenario"] = c.DevelopmentScenario
	}
	if c.Screen == ScreenStartup {
		report["startup_stage"] = c.Startup.Stage
		report["startup_frame"] = c.Startup.Frame
	}
	if c.Mission != nil {
		report["mission_seed"] = c.Mission.Data.Inputs.Seed
		report["mission_objects_sha256"] = c.Mission.Data.ObjectHash
		report["mission_page"] = c.Mission.Page
		report["mission_settings"] = c.Mission.Settings
	}
	if c.Rendering != nil {
		report["rendering_preview"] = c.Rendering.Report()
	}
	if c.SceneEntry != nil {
		report["scene_initialization_stage"] = c.SceneEntry.Stage
	}
	if c.sceneConfirmationPresented() {
		report["scene_end_confirmation"] = c.sceneEndConfirmation
	}
	return writeIndexedFrame(filename, indices, c.CurrentPalette(), c.Office.Assets.Root, report)
}

func writeIndexedFrame(filename string, indices []byte, palette color.Palette, assetsRoot string, report any) error {
	w, h, dimErr := indexedFrameSize(indices)
	if strings.ToLower(filepath.Ext(filename)) != ".png" || dimErr != nil || len(palette) != 256 {
		return fmt.Errorf("frame requires .png, supported indexed dimensions and256 colours")
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	names := []string{filename, stem + ".indices", stem + ".json"}
	for _, name := range names {
		if _, err := assetdata.GeneratedPath(name, assetsRoot); err != nil {
			return err
		}
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	copy(img.Pix, indices)
	pngData, err := assetdata.PngBytes(img)
	if err != nil {
		return err
	}
	meta, err := assetdata.JsonBytes(report)
	if err != nil {
		return err
	}
	for i, data := range [][]byte{pngData, indices, meta} {
		if err = os.MkdirAll(filepath.Dir(names[i]), 0755); err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(filepath.Dir(names[i]), filepath.Base(names[i]), data); err != nil {
			return err
		}
	}
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	decoded, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	result, ok := decoded.(*image.Paletted)
	if !ok || result.Rect != img.Rect || !bytes.Equal(result.Pix, indices) {
		return fmt.Errorf("frame PNG index round trip failed")
	}
	for i, colour := range palette {
		a, b, c, d := colour.RGBA()
		w, x, y, z := result.Palette[i].RGBA()
		if a != w || b != x || c != y || d != z {
			return fmt.Errorf("frame PNG palette round trip failed at%d", i)
		}
	}
	fmt.Printf("frame: %dx%d; indices SHA256 %s; %s\n", w, h, assetdata.Hash(indices), filename)
	return nil
}
