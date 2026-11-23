//go:build !headless

package menus

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"testing"
)

// Native modal ownership: GLFW modifier-make events are not standalone DOS
// response keys and must not commit a page before the rest of Alt+Q arrives.
func TestDebriefNativeModifierOwnership(t *testing.T) {
	for _, key := range []int32{rl.KeyLeftAlt, rl.KeyRightAlt, rl.KeyLeftControl, rl.KeyRightControl, rl.KeyLeftShift, rl.KeyRightShift, rl.KeyLeftSuper, rl.KeyRightSuper, rl.KeyCapsLock, rl.KeyNumLock, rl.KeyScrollLock, 0} {
		if debriefResponseKey(key) {
			t.Fatalf("modifier%d acknowledged result", key)
		}
	}
	for _, key := range []int32{rl.KeyEnter, rl.KeySpace, rl.KeyEscape, rl.KeyA, rl.KeyF1, rl.KeyLeft} {
		if !debriefResponseKey(key) {
			t.Fatalf("ordinary response%d lost", key)
		}
	}
}
