package configuration

import "fmt"

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
