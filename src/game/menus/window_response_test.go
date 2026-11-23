//go:build !headless

package menus

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"testing"
)

func TestAltMakeCannotAcknowledgeBeforeEnter(t *testing.T) {
	for _, alt := range []int32{rl.KeyLeftAlt, rl.KeyRightAlt} {
		if windowResponseMake(alt, false) {
			t.Fatal("Alt alone acknowledged before chord recognition")
		}
	}
	for _, enter := range []int32{rl.KeyEnter, rl.KeyKpEnter} {
		if windowResponseMake(enter, true) || !windowResponseMake(enter, false) {
			t.Fatal("Enter suppression/ordinary response", enter)
		}
	}
	if !windowResponseMake(rl.KeyQ, true) {
		t.Fatal("unrelated command suppressed")
	}
}
