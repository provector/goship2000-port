//go:build !headless

// Package branding owns artwork embedded independently of game asset preparation.
package branding

import (
	_ "embed"

	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed goship2000.png
var gameIcon []byte

//go:embed goship2000-setup.png
var setupIcon []byte

func SetGameIcon()  { setIcon(gameIcon) }
func SetSetupIcon() { setIcon(setupIcon) }

// GLFW copies the supplied pixels before returning.
func setIcon(png []byte) {
	icon := rl.LoadImageFromMemory(".png", png, int32(len(png)))
	if icon != nil && icon.Data != nil {
		rl.SetWindowIcon(*icon)
		rl.UnloadImage(icon)
	}
}

//go:embed splash.png
var Splash []byte

//go:embed skid.png
var Skid []byte
