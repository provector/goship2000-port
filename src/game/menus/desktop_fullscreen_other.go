//go:build !linux && !headless

package menus

import "gunship2000-assets/game/configuration"

type desktopFullscreen struct{}

func openDesktopFullscreen() *desktopFullscreen                  { return nil }
func (*desktopFullscreen) close()                                {}
func (*desktopFullscreen) manages(configuration.WindowMode) bool { return false }
func (*desktopFullscreen) fullscreen() bool                      { return false }
func (*desktopFullscreen) request(bool)                          {}

func (*desktopFullscreen) clientRect(*displayWindow) {}

func (*desktopFullscreen) rememberFrame()       {}
func (*desktopFullscreen) restoringFrame() bool { return false }
