package main

import (
	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
)

// Application orchestration supplies audio conversion without an assets→sounds
// dependency; failure remains inside the same staging/promotion boundary.
func prepareAssets(input, output string) (*assetdata.Manifest, error) {
	return assetdata.PrepareAssetsWithAudio(input, output, sound.ExportAudioMetadata)
}
