package managedassets

import (
	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
	"gunship2000-assets/internal/audioopl"
	"os"
)

func FrontEndServices(progress func(assetdata.PreparationProgress)) Services {
	var work assetdata.PreparationWork
	emit := func(p assetdata.PreparationProgress) {
		if progress != nil {
			progress(work.Observe(p))
		}
	}
	return Services{Installations: InstalledGames, Pack: assetdata.PackPrepared, Prepare: func(in, out string) (*assetdata.Manifest, error) {
		if err := audioopl.PrepareHelper(); err != nil {
			return nil, err
		}
		return assetdata.PrepareAssetsWithProgress(in, out, func(root string, m *assetdata.Manifest) error {
			return sound.ExportAudioMetadataProgress(root, m, emit)
		}, emit)
	}, Load: assetdata.LoadFrontEndAssets, Ready: assetdata.ValidateCurrentFrontEndCapabilities, Rename: os.Rename, Progress: emit}
}
