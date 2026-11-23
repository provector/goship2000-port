//go:build !headless

package nativeaudio

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
)

// Raylib/miniaudio requests PCM from its audio thread. Keeping the intro and
// loop in one stream avoids render-frame polling or a stop/start loop gap.
func OpenAudioOutput(player *sound.AudioPlayer) (func(), error) {
	rl.InitAudioDevice()
	if !rl.IsAudioDeviceReady() {
		return nil, fmt.Errorf("audio device unavailable; continuing silently")
	}
	stream := rl.LoadAudioStream(assetdata.AudioSampleRate, 32, 1)
	if stream.Buffer == nil {
		rl.CloseAudioDevice()
		return nil, fmt.Errorf("audio PCM stream unavailable; continuing silently")
	}
	player.SetDeviceClock(true)
	rl.SetAudioStreamCallback(stream, player.Render)
	rl.PlayAudioStream(stream)
	return func() {
		player.Stop()
		rl.StopAudioStream(stream)
		rl.UnloadAudioStream(stream)
		rl.CloseAudioDevice()
	}, nil
}
