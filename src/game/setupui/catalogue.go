package setupui

import (
	"fmt"
	"gunship2000-assets/game/configuration"
	"slices"
	"strconv"
	"strings"
)

func integer(id string, p func(*configuration.Config) *int) Setting {
	return Setting{ID: id, Kind: "input", Get: func(c *configuration.Config) string { return strconv.Itoa(*p(c)) }, Set: func(c *configuration.Config, v string) error {
		n, e := strconv.Atoi(v)
		if e != nil {
			return fmt.Errorf("%s: integer required", id)
		}
		*p(c) = n
		return nil
	}}
}
func toggle(id string, p func(*configuration.Config) *bool) Setting {
	return Setting{ID: id, Kind: "toggle", Get: func(c *configuration.Config) string {
		if *p(c) {
			return "On"
		}
		return "Off"
	}, Set: func(c *configuration.Config, _ string) error { *p(c) = !*p(c); return nil }}
}
func text(id string, p func(*configuration.Config) *string) Setting {
	return Setting{ID: id, Kind: "input", Get: func(c *configuration.Config) string { return *p(c) }, Set: func(c *configuration.Config, v string) error { *p(c) = v; return nil }}
}
func initialWindowSetting() Setting {
	choices := configuration.WindowResolutions()
	for i := range choices {
		choices[i] = strings.ReplaceAll(choices[i], "x", "×")
	}
	return Setting{ID: "initial_window", Kind: "choice", Choices: choices,
		Get: func(c *configuration.Config) string {
			return fmt.Sprintf("%d×%d", c.WindowWidth, c.WindowHeight)
		},
		Set: func(c *configuration.Config, choice string) error {
			if !slices.Contains(choices, choice) {
				return fmt.Errorf("unknown initial window size %q", choice)
			}
			_, err := fmt.Sscanf(choice, "%d×%d", &c.WindowWidth, &c.WindowHeight)
			return err
		},
	}
}

func Catalogue(menu string) []Setting {
	var a []Setting
	switch menu {
	case "graphics":
		a = []Setting{

			{ID: "render_scale", Kind: "choice", Choices: []string{"1", "2", "800x600", "1024x768", "1280x1024"}, Get: func(c *configuration.Config) string {
				switch c.Runtime.SimulationResolution {
				case "320x200":
					return "1"
				case "640x480":
					return "2"
				}
				if c.Runtime.SimulationResolution != "" {
					return c.Runtime.SimulationResolution
				}
				return strconv.Itoa(c.Runtime.SimulationRenderScale)
			}, Set: func(c *configuration.Config, v string) error {
				switch v {
				case "1":
					v = "320x200"
				case "2":
					v = "640x480"
				}
				return c.Runtime.SetResolution(v)
			}},
			toggle("fullscreen", func(c *configuration.Config) *bool { return &c.Fullscreen }),
			{ID: "presentation", Kind: "cycle", Choices: []string{"stepped", "smooth"}, Get: func(c *configuration.Config) string { return c.Runtime.PresentationMode }, Set: func(c *configuration.Config, v string) error {
				c.Runtime.SetFramerate(v)
				return nil
			}},
			{ID: "floating", Kind: "cycle", Choices: []string{"off", "on"}, Get: func(c *configuration.Config) string {
				if c.Runtime.FloatingGeometry {
					return "on"
				}
				return "off"
			}, Set: func(c *configuration.Config, value string) error {
				c.Runtime.FloatingGeometry = value == "on"
				return nil
			}}}
		a = append(a, initialWindowSetting(), Setting{ID: "draw_fps_cpu", Kind: "toggle", Get: func(c *configuration.Config) string {
			if c.DrawFPSCPU {
				return "True"
			}
			return "False"
		}, Set: func(c *configuration.Config, _ string) error { c.DrawFPSCPU = !c.DrawFPSCPU; return nil }}, Setting{ID: "legacy_defaults", Kind: "preset"}, Setting{ID: "modern_defaults", Kind: "preset"})
	case "sound":
		a = []Setting{
			{ID: "effects_enabled", Kind: "toggle", Get: func(c *configuration.Config) string {
				if c.SoundEffectsEnabled && c.SoundEffectsVolume > 0 {
					return "On"
				}
				return "Off"
			}, Set: func(c *configuration.Config, _ string) error { c.ToggleChannel(false); return nil }},
			{ID: "effects_volume", Kind: "level", Get: func(c *configuration.Config) string { return strconv.Itoa(c.SoundEffectsVolume) }, Set: func(c *configuration.Config, v string) error {
				n, e := strconv.Atoi(v)
				if e != nil {
					return e
				}
				if n < 0 || n > 100 {
					return fmt.Errorf("effects_volume: must be0..100")
				}
				c.SetVolume(false, n)
				return nil
			}},
			{ID: "music_enabled", Kind: "toggle", Get: func(c *configuration.Config) string {
				if c.MusicEnabled && c.MusicVolume > 0 && c.Audio.MusicMode != configuration.MusicOff {
					return "On"
				}
				return "Off"
			}, Set: func(c *configuration.Config, _ string) error { c.ToggleChannel(true); return nil }},
			{ID: "music_volume", Kind: "level", Get: func(c *configuration.Config) string { return strconv.Itoa(c.MusicVolume) }, Set: func(c *configuration.Config, v string) error {
				n, e := strconv.Atoi(v)
				if e != nil {
					return e
				}
				if n < 0 || n > 100 {
					return fmt.Errorf("music_volume: must be0..100")
				}
				c.SetVolume(true, n)
				return nil
			}},
			toggle("speech", func(c *configuration.Config) *bool { return &c.SpeechEnabled })}
	case "controls":
		a = []Setting{{ID: "joystick", Kind: "device"}, {ID: "basic_controls", Kind: "basic"}, {ID: "advanced_controls", Kind: "controller_advanced"}, {ID: "collective_mode", Kind: "collective_mode"}, {ID: "collective_source_override", Kind: "collective_source"}, {ID: "reset_bindings", Kind: "reset_bindings"}}
	case "settings":
		a = []Setting{text("original", func(c *configuration.Config) *string { return &c.Original }), {ID: "saves_location", Kind: "location"}, integer("recording", func(c *configuration.Config) *int { return &c.ReplayMaxMiB }), {ID: "mouse_capture", Kind: "choice", Choices: []string{"click", "disabled"}, Get: func(c *configuration.Config) string { return c.Runtime.MouseCapture }, Set: func(c *configuration.Config, v string) error { c.Runtime.MouseCapture = v; return nil }},
			{ID: "avoidance_policy", Kind: "choice", Choices: []string{"legacy", "corrected"}, Get: func(c *configuration.Config) string { return string(c.Runtime.AvoidancePolicy) }, Set: func(c *configuration.Config, v string) error {
				c.Runtime.AvoidancePolicy = configuration.AvoidancePolicy(v)
				return nil
			}}, toggle("field_pattern", func(c *configuration.Config) *bool { return &c.SimplifiedFields }), toggle("legacy_copy_protection", func(c *configuration.Config) *bool { return &c.LegacyCopyProtection })}
	case "debug":
		a = []Setting{
			toggle("enemy_ai", func(c *configuration.Config) *bool { return &c.Runtime.DisableEnemyAI }),
			toggle("wingmen_ai", func(c *configuration.Config) *bool { return &c.Runtime.DisableWingmenAI }),
			{ID: "advanced", Kind: "advanced"},
			{ID: "logging", Kind: "logging"},
		}
	case "logging":
		a = []Setting{
			toggle("show_log", func(c *configuration.Config) *bool { return &c.Logging.ShowLog }),
			{ID: "log_level", Kind: "choice", Choices: []string{"DEBUG", "INFO", "WARN", "ERROR"}, Get: func(c *configuration.Config) string { return c.Logging.Level }, Set: func(c *configuration.Config, v string) error { c.Logging.Level = v; return nil }},
			toggle("write_log", func(c *configuration.Config) *bool { return &c.Logging.WriteLog }),
			integer("log_limit", func(c *configuration.Config) *int { return &c.Logging.FileLimitMB }),
			text("log_name", func(c *configuration.Config) *string { return &c.Logging.FileName }),
			{ID: "terminal_target", Kind: "cycle", Choices: []string{"system", "custom"}, Get: func(c *configuration.Config) string { return c.Logging.TerminalTarget }, Set: func(c *configuration.Config, v string) error { c.Logging.TerminalTarget = v; return nil }},
		}
	case "advanced":
		a = []Setting{
			integer("fade", func(c *configuration.Config) *int { return &c.FadeMilliseconds }),
			text("assets", func(c *configuration.Config) *string { return &c.Assets }), text("saves", func(c *configuration.Config) *string { return &c.Saves }),
			toggle("square_pixels", func(c *configuration.Config) *bool { return &c.SquarePixels }),
			{ID: "music_mode", Kind: "choice", Choices: []string{"all", "background_off", "off"}, Get: func(c *configuration.Config) string { return string(c.Audio.MusicMode) }, Set: func(c *configuration.Config, v string) error {
				c.Audio.MusicMode = configuration.MusicMode(v)
				return nil
			}},
			integer("gps_divisor", func(c *configuration.Config) *int { return &c.Runtime.GPSOverviewKeyboardDivisor }), integer("repeat_delay", func(c *configuration.Config) *int { return &c.Runtime.CollectiveRepeatDelayMS }), integer("repeat_interval", func(c *configuration.Config) *int { return &c.Runtime.CollectiveRepeatIntervalMS }), integer("queue", func(c *configuration.Config) *int { return &c.Runtime.CommandQueueCapacity }),
			{ID: "dev_seed", Kind: "input", Get: func(c *configuration.Config) string { return strconv.FormatUint(c.Development.Seed, 10) }, Set: func(c *configuration.Config, v string) error {
				n, e := strconv.ParseUint(v, 10, 64)
				c.Development.Seed = n
				return e
			}},
			{ID: "tick_rate", Kind: "input", Get: func(c *configuration.Config) string { return fmt.Sprint(c.Development.TickRate) }, Set: func(c *configuration.Config, v string) error {
				n, e := strconv.ParseFloat(v, 64)
				c.Development.TickRate = n
				return e
			}},
			{ID: "preview_rate", Kind: "input", Get: func(c *configuration.Config) string { return fmt.Sprint(c.Development.PreviewRate) }, Set: func(c *configuration.Config, v string) error {
				n, e := strconv.ParseFloat(v, 64)
				c.Development.PreviewRate = n
				return e
			}},
			integer("arrangement", func(c *configuration.Config) *int { return &c.Development.Arrangement }), integer("cockpit_aircraft", func(c *configuration.Config) *int { return &c.Development.CockpitAircraft }), toggle("pointer", func(c *configuration.Config) *bool { return &c.Development.Pointer }), integer("pointer_x", func(c *configuration.Config) *int { return &c.Development.PointerX }), integer("pointer_y", func(c *configuration.Config) *int { return &c.Development.PointerY }), toggle("visual_paused", func(c *configuration.Config) *bool { return &c.Development.VisualPaused }), {ID: "cli_commands", Kind: "commands"}}
	}
	return append([]Setting{{ID: "back", Kind: "back"}}, a...)
}
