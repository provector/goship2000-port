package menus

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Set by the reproducible build script. Unmarked go-run/IDE builds must not
// pretend to have the source identity of a previously packaged binary.
func DiagnosticBuildIdentity() string { return filmBuildIdentity() }

var buildSourceIdentity = "unrecorded"

// Developer launches may have no injected source stamp. Hash the actual running
// image once, never mutable checkout files, and label the identity domain.
var filmBuildIdentity = sync.OnceValue(func() string {
	if buildSourceIdentity != "" && buildSourceIdentity != "unrecorded" {
		return buildSourceIdentity
	}
	path, err := os.Executable()
	if err != nil {
		return "unrecorded"
	}
	if runtime.GOOS == "linux" {
		path = "/proc/self/exe"
	}
	f, err := os.Open(path)
	if err != nil {
		return "unrecorded"
	}
	defer f.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, f); err != nil {
		return "unrecorded"
	}
	return fmt.Sprintf("exe-sha256:%x", digest.Sum(nil))
})

func developmentIdentity(assets string) map[string]any {
	identity := map[string]any{"build_source_sha256": buildSourceIdentity, "go_version": runtime.Version(), "assets": assets}
	if exe, err := os.Executable(); err == nil {
		identity["executable"] = exe
		if raw, err := assetdata.ReadLimitedFile(exe, 128<<20); err == nil {
			identity["executable_sha256"] = assetdata.Hash(raw)
		}
	}
	if raw, err := assetdata.ReadLimitedFile(filepath.Join(assets, "manifest.json"), 16<<20); err == nil {
		identity["asset_manifest_sha256"] = assetdata.Hash(raw)
		var manifest struct {
			Schema      int `json:"schema_version"`
			Preparation int `json:"preparation_version"`
		}
		if json.Unmarshal(raw, &manifest) == nil {
			identity["asset_schema"] = manifest.Schema
			identity["asset_preparation_version"] = manifest.Preparation
		}
	}
	identity["supported_source_profile"] = assetdata.IslandIce469085.Name
	identity["supported_original_executable_sha256"] = assetdata.IslandIce469085.Source(assetdata.RoleMainExecutable).SHA256
	identity["preparation_version_supported"] = assetdata.PreparationVersion
	return identity
}

func writeDevelopmentCheckpoint(directory string, frame int, c *FrontEnd) error {
	report := map[string]any{"identity": developmentIdentity(c.Office.Assets.Root), "screen": c.Screen,
		"save_revision": c.Store.State().Revision, "scratch_save": c.Store.Directory,
		"scope": "Own native port; programmatic capture, not physical or original-game validation"}
	if s := c.Mission; s != nil {
		report["mission"] = s.Data
		report["page"] = s.Page
		report["settings"] = s.Settings
		report["flight"] = s.Flight
		report["aircraft_rows"] = s.ReadinessRows
		if v := s.Ordnance; v != nil {
			report["ordnance"] = map[string]any{"working": v.Working, "hover": v.Hover, "choices": v.Choices}
		}
	}
	if c.SceneEntry != nil {
		report["scene_initialization_stage"] = c.SceneEntry.Stage
	}
	if c.sceneConfirmationPresented() {
		report["scene_end_confirmation"] = c.sceneEndConfirmation
	}
	if c.Rendering != nil {
		report["rendering_preview"] = c.Rendering.Report()
	}
	raw, err := assetdata.JsonBytes(report)
	if err != nil {
		return err
	}
	_, err = assetdata.WriteNew(directory, fmt.Sprintf("window-%03d.session.json", frame), raw)
	return err
}
