package sounds

import (
	"gunship2000-assets/game/diagnostics"
	"io"
	"log/slog"
)

// Small injected seam for missing-device tests; the normal caller always uses
// the existing native audio backend. Failure is silent playback, not startup loss.
func StartAudioOutput(player *AudioPlayer, open func(*AudioPlayer) (func(), error), log io.Writer) func() {
	cleanup, err := open(player)
	if err != nil {
		player.SetDeviceClock(false)
		diagnostics.Existing(slog.LevelWarn, "audio", "audio", err, log)
		diagnostics.Info("audio", "Audio device unavailable; silent clock fallback active")
		return player.Stop
	}
	return cleanup
}
