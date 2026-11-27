//go:build !headless

package menus

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	sound "gunship2000-assets/game/sounds"
	"image"
	"slices"
	"time"
)

const pointFade = 150 * time.Millisecond

type pointWindow struct {
	open, loading, restored bool
	busy, presented         bool

	image, before *image.RGBA
	transition    time.Time
	fadeIn        bool
}

// Claim the chord even when a snapshot is unavailable (GPS, Outfit, replay).
// A released Alt must not turn the still-held L or a buffered character into
// a Land/Loadout action.
func (g *graphicsWindow) takePointShortcut(keys []int32, alt bool) ([]int32, bool) {
	if !alt || !slices.Contains(keys, int32(rl.KeyL)) {
		return keys, false
	}
	if g.consumed == nil {
		g.consumed = make(map[int32]bool)
	}
	g.consumed[rl.KeyL] = true
	return slices.DeleteFunc(keys, func(k int32) bool { return k == rl.KeyL }), true
}

func (s *pointWindow) input(c *FrontEnd, keys []int32, requested, closing bool, graphics *graphicsWindow) (bool, string) {
	now := time.Now()
	if s.fadeIn {
		if now.Sub(s.transition) >= pointFade {
			if s.before != nil {
				s.before = nil
				s.transition = now
			} else {
				*s = pointWindow{}
				graphics.consumeHeldPointKeys()
			}
		}
		return true, ""
	}
	if !s.open {
		if closing || graphics.menu != nil || !requested || !rl.IsWindowFocused() || !c.pointAvailable() {
			return false, ""
		}
		s.open = true
		s.loading = c.Rendering.sessionPoint != nil
		return true, ""
	}
	if closing || slices.Contains(keys, int32(rl.KeyEscape)) {
		*s = pointWindow{}
		graphics.consumeHeldPointKeys()
		return true, ""
	}
	if !s.busy {
		if slices.Contains(keys, int32(rl.KeyEnter)) || slices.Contains(keys, int32(rl.KeyKpEnter)) {
			s.busy, s.image = true, nil
		}
		return true, ""
	}
	// EndDrawing acknowledges the busy panel before copying the mission.
	if !s.presented {
		return true, ""
	}
	if e := c.saveOrLoadPoint(); e != nil {
		return s.fail(graphics, e)
	}
	if s.loading {
		*s = pointWindow{before: s.before, transition: time.Now(), fadeIn: true, restored: true}
	} else {
		*s = pointWindow{}
	}
	graphics.consumeHeldPointKeys()
	return true, ""
}

func (s *pointWindow) fail(graphics *graphicsWindow, e error) (bool, string) {
	*s = pointWindow{}
	graphics.consumeHeldPointKeys()
	return true, fmt.Sprintf("Mission point unavailable: %v", e)
}

func (s *pointWindow) markPresented() {
	if s.open && s.busy {
		s.presented = true
	}
}

func (s *pointWindow) alpha() uint8 {
	if !s.fadeIn {
		return 0
	}
	phase := min(1.0, time.Since(s.transition).Seconds()/pointFade.Seconds())
	if s.before != nil {
		return uint8(255 * phase)
	}
	return uint8(255 * (1 - phase))
}

func (s *pointWindow) compose(c *FrontEnd) (*image.RGBA, error) {
	if s.image != nil {
		return s.image, nil
	}
	pixels := slices.Clone(c.Frame(false))
	if e := c.Rendering.Error; e != nil {
		return nil, e
	}
	lines := []string{"You have only 1 save left! Use it wisely.", "Press ENTER to Save", "Press Esc to Cancel"}
	if s.loading && s.busy && s.before == nil {
		before, e := indexedFrameRGBAInto(pixels, c.CurrentPalette(), nil)
		if e != nil {
			return nil, e
		}
		c.applySkyPresentation(before, pixels)
		s.before = before
	}
	if s.loading {
		lines = []string{"You have previous state saved.", "Press ENTER to Load", "Current progress will be lost", "Press Esc to cancel"}
	}
	busy := ""
	if s.busy {
		busy = "Saving..."
		if s.loading {
			busy = "Loading..."
		}
	}
	if e := c.Office.Assets.Cockpit.DrawPointMenu(pixels, lines, busy, c.Rendering.Scene.Scale()); e != nil {
		return nil, e
	}
	img, e := indexedFrameRGBAInto(pixels, c.CurrentPalette(), nil)
	if e != nil {
		return nil, e
	}
	c.applySkyPresentation(img, pixels)
	s.image = img
	return img, nil
}

// Drop audio belonging to the abandoned continuation once, at successful handover.
func (s *pointWindow) syncRestoredAudio(audio *sound.AudioPlayer, pitch uint16) {
	if !s.restored {
		return
	}
	s.restored = false
	if audio != nil {
		audio.RestoreFlightEffects(pitch)
	}
}
