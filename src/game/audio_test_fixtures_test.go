package main

import (
	assetdata "gunship2000-assets/game/assets"
)

func audioTestAssets() *assetdata.AudioAssets {
	clip := func(xs ...int16) *assetdata.AudioClip {
		return &assetdata.AudioClip{Samples: xs, SampleRate: assetdata.AudioSampleRate, BusyEndFrame: uint64(len(xs))}
	}
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{"logo": clip(100, 200, 0), "title": clip(300, 400, 0, 0), "office_intro": clip(1000, 2000), "office_loop": clip(3000, 4000), "gs4_0d": clip(600, 700), "gs4_22": clip(100, 200, 0)}}
	a.Clips["title"].BusyEndFrame = 2
	return a
}
