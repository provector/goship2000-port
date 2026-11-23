package sounds

import (
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"path/filepath"
)

func ExportAudioMetadata(root string, m *assetdata.Manifest) error {
	return ExportAudioMetadataProgress(root, m, nil)
}
func ExportAudioMetadataProgress(root string, m *assetdata.Manifest, progress func(assetdata.PreparationProgress)) error {
	p, err := recognizeAudioProfile(m.Resources)
	if err != nil {
		m.UnsupportedVersionOperations = append(m.UnsupportedVersionOperations, assetdata.UnsupportedVersionOperation{"startup/front-end audio", assetdata.IslandIce469085.Name, err.Error()})
		return nil
	}
	m.AudioMetadata, err = prepareAudioResourcesProgress(root, m.Resources, p, func(n, total int) {
		if progress != nil {
			progress(assetdata.PreparationProgress{Stage: "Sound conversion", Completed: n, Total: total})
		}
	})
	if err != nil {
		return fmt.Errorf("offline audio preparation: %w", err)
	}
	raw, err := assetdata.ReadLimitedFile(filepath.Join(root, filepath.FromSlash(m.AudioMetadata)), 8<<20)
	if err != nil {
		return err
	}
	var a assetdata.AudioMetadata
	if err = json.Unmarshal(raw, &a); err != nil {
		return err
	}
	for _, role := range assetdata.AudioSourceRoles {
		r, e := audioResource(m.Resources, p, role)
		if e != nil {
			return e
		}
		if r.Conversion.Status == "unsupported" {
			if m.Summary.Unsupported > 0 {
				m.Summary.Unsupported--
			}
			if m.Summary.UnsupportedFormats[r.Format] > 1 {
				m.Summary.UnsupportedFormats[r.Format]--
			} else {
				delete(m.Summary.UnsupportedFormats, r.Format)
			}
		}
		r.Conversion.Status = "converted_audio"
		r.Conversion.RuntimeValidation = "Canonical source-derived audio; qualified Step27 source/CPU/listening checks, not universal runtime equivalence."
		if role == assetdata.RoleAudioADigital {
			r.Conversion.RuntimeValidation = "Digital ranges retain Step27 checks; GS2 initialization FM is source/hand-checked with explicit canonical early-loop and bounded later-ramp policies, not original audible validation."
		}
	}
	for _, t := range a.Tracks {
		if t.Bits == 16 {
			m.Summary.AudioFMTracks++
		} else {
			m.Summary.AudioDigitalTracks++
		}
	}
	return nil
}
