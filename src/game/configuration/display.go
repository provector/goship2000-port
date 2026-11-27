package configuration

import (
	"fmt"
	"slices"
)

// WindowMode concerns the OS game window only. It is outside recorded runtime options.
type WindowMode uint8

const (
	Windowed WindowMode = iota
	Borderless
	FullScreen
)
const BorderlessLabel = "Borderless F"
const FullScreenLabel = "FullScreen"

func WindowChoices() []string { return append(WindowResolutions(), BorderlessLabel, FullScreenLabel) }
func (c Config) WindowMode() WindowMode {
	if !c.Fullscreen {
		return Windowed
	}
	if c.BorderlessFullscreen {
		return Borderless
	}
	return FullScreen
}
func (m WindowMode) Label(w, h int) string {
	switch m {
	case Borderless:
		return BorderlessLabel
	case FullScreen:
		return FullScreenLabel
	}
	return fmt.Sprintf("%dx%d", w, h)
}
func (c *Config) SetWindowMode(mode WindowMode) {
	c.Fullscreen = mode != Windowed
	c.BorderlessFullscreen = mode == Borderless
}
func (c *Config) SetWindowChoice(value string) error {
	switch value {
	case BorderlessLabel:
		c.SetWindowMode(Borderless)
	case FullScreenLabel:
		c.SetWindowMode(FullScreen)
	default:
		if !slices.Contains(WindowResolutions(), value) {
			return fmt.Errorf("unsupported window size %q", value)
		}
		fmt.Sscanf(value, "%dx%d", &c.WindowWidth, &c.WindowHeight)
		c.SetWindowMode(Windowed)
	}
	return nil
}

func DisplayResolutions() []string {
	return []string{"320x200", "640x480", "800x600", "1024x768", "1280x1024"}
}
func WindowResolutions() []string { return DisplayResolutions()[1:] }

func (o *RuntimeOptions) SetResolution(value string) error {
	switch value {
	case "320x200":
		o.SimulationRenderScale = 1
	case "640x480":
		o.SimulationRenderScale = 2
	case "800x600", "1024x768", "1280x1024":
	default:
		return fmt.Errorf("unsupported internal resolution %q", value)
	}
	o.SimulationResolution = value
	return nil
}
func (o *RuntimeOptions) SetFramerate(mode string) { o.PresentationMode = mode; o.PresentationFPS = 60 }
