package instancealert

import (
	"fmt"
	"os/exec"
	"strings"
)

// Supply title and message as separate arguments. Paths and diagnostics may
// contain quotes or newlines, and must never be interpreted as AppleScript.
func dialog(title, message string) error {
	cmd := exec.Command("/usr/bin/osascript", "-", title, message)
	cmd.Stdin = strings.NewReader(`on run argv
display alert (item 1 of argv) message (item 2 of argv) as critical buttons {"OK"} default button "OK"
end run`)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("system alert: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
