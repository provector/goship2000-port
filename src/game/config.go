package main

import (
	"gunship2000-assets/game/configuration"
	sound "gunship2000-assets/game/sounds"
)

// Installed builds set this with -ldflags '-X main.applicationMode=installed'.
// Go's build info has no go-run bit; an exact current-directory source check
// also supports unmarked IDE builds without guessing Go's temporary paths.
var applicationMode = "auto"

type AppConfig configuration.Config
type AudioConfig = configuration.AudioConfig

func (c AppConfig) audioPreferences() *sound.AudioPreferences {
	// Defaults are applied before JSON decoding: explicit zero stays silent.
	return &sound.AudioPreferences{MusicMode: c.Audio.MusicMode, MusicVolume: float64(c.MusicVolume) / 100, EffectsVolume: float64(c.SoundEffectsVolume) / 100, SpeechMuted: !c.SpeechEnabled}
}

type ApplicationPaths struct {
	RecordingLimit                     configuration.RecordingLimit
	Directory, Original, Assets, Saves string
	Installation                       string
	AssetsOrigin                       string
	Config                             AppConfig
}

func appRelative(directory, name string) string { return configuration.Relative(directory, name) }
func applicationDirectory(override, cwd, executable, module, mode string) (string, error) {
	return configuration.ApplicationDirectory(override, cwd, executable, module, mode)
}
func resolveApplication(override, assets string) (ApplicationPaths, error) {
	dir, e := configuration.ResolveDirectory(override, applicationMode)
	if e != nil {
		return ApplicationPaths{}, e
	}
	return loadApplication(dir, assets)
}
func (p ApplicationPaths) install() string {
	if p.Installation != "" {
		return p.Installation
	}
	return p.Directory
}
func loadApplication(dir, assets string) (ApplicationPaths, error) {
	return loadApplicationAt(dir, dir, assets)
}
func loadApplicationAt(dir, data, assets string) (ApplicationPaths, error) {
	p, e := configuration.LoadApplicationAt(dir, data, assets)
	if e != nil {
		return ApplicationPaths{}, e
	}
	return ApplicationPaths{p.RecordingLimit, data, p.Original, p.Assets, p.Saves, dir, p.AssetsOrigin, AppConfig(p.Config)}, nil
}
