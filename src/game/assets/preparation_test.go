package assets

func prepareAssets(input, output string) (*Manifest, error) {
	return PrepareAssetsWithAudio(input, output, nil)
}
