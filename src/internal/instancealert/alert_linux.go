package instancealert

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func dialog(title, message string) error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errors.New("no graphical display")
	}
	for _, args := range [][]string{{"zenity", "--error", "--title", title, "--text", message, "--no-markup", "--width", "720", "--ok-label", "OK"}, {"kdialog", "--title", title, "--error", message}, {"xmessage", "-title", title, "-buttons", "OK:0", "-geometry", "720x300", "-center", message}} {
		path, e := exec.LookPath(args[0])
		if e != nil {
			continue
		}
		out, e := exec.Command(path, args[1:]...).CombinedOutput()
		if e == nil {
			return nil
		}
		// A user closing a dialog can return status 1. Avoid displaying a
		// second box unless the helper actually failed to open.
		var status *exec.ExitError
		if errors.As(e, &status) && status.ExitCode() == 1 && strings.TrimSpace(string(out)) == "" {
			return nil
		}
	}
	return errors.New("no system dialog available")
}
