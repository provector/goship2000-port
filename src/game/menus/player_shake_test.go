package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/internal/testfixture"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPlayerShakeOriginalOperands(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("original static operands")
	}
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("source identity")
	}
	for at, want := range map[int]string{0xd8b0 + 0x94a9: "26c706cd0a1000", 0xd8b0 + 0x2f2a: "26ff0ecd0a7421", 0xd8b0 + 0x2f31: "f606da0001", 0xd8b0 + 0x2f3c: "26833ed10a00", 0xd8b0 + 0x2f48: "26a1cd0ac1f802", 0x3e550 + 0x78: "eab30c471d", 0x3e550 + 0xcb6: "8b46068ae032c0"} {
		v, _ := hex.DecodeString(want)
		if !bytes.Equal(b[at:at+len(v)], v) {
			t.Fatalf("source operand%x", at)
		}
	}
}

func TestPlayerShakeDisplaySequenceAndBothScales(t *testing.T) {
	var latch playerShakePresentation
	for _, v := range []struct {
		remaining  int16
		phase      uint16
		view, want int
	}{{15, 2, 0, 3}, {14, 3, 0, 0}, {13, 4, 0, 3}, {12, 6, 7, 3}, {12, 7, 7, 0}, {8, 8, 3, 0}, {7, 8, 0, 1}, {0, 10, 0, 0}} {
		latch.advance(v.remaining, v.phase, v.view)
		if int(latch.rows) != v.want {
			t.Fatal(v, latch)
		}
	}
	for _, scale := range []int{1, 2} {
		w, h := 320*scale, 200*scale
		original := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				original[y*w+x] = byte((y*13+x*7)%251 + 1)
			}
		}
		for _, fps := range []int{10, 60} {
			latch.rows = 3
			for frame := 0; frame < fps; frame++ {
				dst := append([]byte(nil), original...)
				if err := latch.draw(dst, scale); err != nil {
					t.Fatal(err)
				}
				n := 3 * scale * w
				if !bytes.Equal(dst[:len(dst)-n], original[n:]) || !bytes.Equal(dst[len(dst)-n:], make([]byte, n)) {
					t.Fatal("source whole-image display-start", scale, fps, frame)
				}
				if latch.rows != 3 {
					t.Fatal("redraw advanced logical effect")
				}
			}
		}
		latch.rows = 205 // VGA low16(205*320)=64; preserves subpixels at genuine2x.
		dst := append([]byte(nil), original...)
		if err := latch.draw(dst, scale); err != nil {
			t.Fatal(err)
		}
		if dst[0] != original[64*scale] || scale == 2 && dst[1] != original[129] {
			t.Fatal("VGA wrapped address/subpixel", scale, dst[:2])
		}
	}
}

func TestPlayerShakeComposedFrameAndCanonicalIsolation(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := developmentScenarioController(t, "fuel-empty")
		p := c.Rendering
		p.Scene.RenderScale = scale
		p.Frame = make([]byte, p.Scene.TargetPixels())
		p.worldFrame = make([]byte, p.Scene.TargetPixels())
		p.resetPresentation()
		p.playerShake.rows = 0
		p.Dirty = true
		c.Frame(false)
		before := append([]byte(nil), p.Frame...)
		shakeCapture(t, p, fmt.Sprintf("before-%dx", scale), before)
		state := p.FilmCheckpoint()
		canonical := append([]byte(nil), p.avionicsReference...)
		p.playerShake.advance(12, 2, 0)
		p.Dirty = true
		c.Frame(false)
		after := p.Frame
		shakeCapture(t, p, fmt.Sprintf("shift-%dx", scale), after)
		n := 3 * scale * 320 * scale
		if p.Error != nil || !bytes.Equal(after[:len(after)-n], before[n:]) {
			t.Fatal("composed cockpit/HUD did not reach CRTC presentation", scale, p.Error)
		}
		if !reflect.DeepEqual(p.FilmCheckpoint(), state) || !bytes.Equal(p.avionicsReference, canonical) {
			t.Fatal("presentation shake changed canonical feedback or checkpoint", scale)
		}
		p.playerShake.advance(0, 4, 0)
		p.Dirty = true
		c.Frame(false)
		shakeCapture(t, p, fmt.Sprintf("reset-%dx", scale), p.Frame)
		if !bytes.Equal(p.Frame, before) {
			t.Fatal("source display-start reset did not restore view")
		}
	}
}

func TestAcceptedPlayerContactReachesNextPrefixDisplay(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			c := developmentScenarioController(t, "fuel-empty")
			p, r := c.Rendering, c.Rendering.Flight
			p.Initialized.Switches |= 0x400 // Damage protection is after accepted shake.
			p.Initialized.RandomState = 0x280
			p.Scene.Environment.RNG = 0x280
			r.Systems.Fuel[0] = 100
			r.State.Shake = 0
			shot := combat.Projectile{Flags: 3, Lifetime: 10, Shooter: 1, Target: 0, Speed: 64, Pose: p.Initialized.Entities[0].Pose}
			if reject {
				shot.Packed = 255
			}
			shot.Pose.H1 -= 64
			if err := r.Combat.SupplyIncoming(20, shot, true); err != nil {
				t.Fatal(err)
			}
			if err := r.Combat.Step(p.combatInput(6, 1, p.nextGameplayRandom)); err != nil {
				t.Fatal(err)
			}
			p.collectCombatEvents()
			if reject {
				if r.State.Shake != 0 || p.playerShake.rows != 0 {
					t.Fatal("rejected contact shook player")
				}
				return
			}
			if r.State.Shake != 16 || p.playerShake.rows != 0 {
				t.Fatal("late contact must set ACD16 without retroactive prefix display", r.State.Shake, p.playerShake)
			}
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 10000 && p.playerShake.rows == 0; i++ {
				if err := c.advanceFlight(.001); err != nil {
					t.Fatal(err)
				}
			}
			if p.playerShake.rows == 0 || r.State.Shake >= 16 {
				t.Fatal("next reached even/cockpit prefix did not publish display-start", r.State.Shake, p.playerShake)
			}
			before := p.FilmCheckpoint()
			c.Frame(false)
			c.Frame(false)
			if !reflect.DeepEqual(p.FilmCheckpoint(), before) {
				t.Fatal("hit draw changed simulation")
			}
		})
	}
}

func shakeCapture(t *testing.T, p *RenderingPreview, name string, pixels []byte) {
	t.Helper()
	dir := os.Getenv("GS2000_R221_CONTACT_CAPTURE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	scale := p.Scene.Scale()
	im := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), p.Palette)
	copy(im.Pix, pixels)
	f, err := os.OpenFile(filepath.Join(dir, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerShakeClockSequenceAt10And60FPS(t *testing.T) {
	type visit struct {
		phase           uint16
		remaining, rows int16
	}
	var expected []visit
	for _, fps := range []int{10, 60} {
		var clock flight.Clock
		var latch playerShakePresentation
		remaining := int16(16)
		visits := []visit{}
		frames, ticks := 0, 0
		for ; frames < fps*10 && remaining != 0; frames++ {
			for _, step := range clock.Advance(time.Second/time.Duration(fps), false) {
				ticks++
				if step.E0 && remaining != 0 {
					remaining--
					latch.advance(remaining, step.Phase, 0)
					visits = append(visits, visit{step.Phase, remaining, latch.rows})
				}
			}
		}
		if len(visits) != 16 || latch.rows != 0 {
			t.Fatal("source sixteen E0 visits did not settle", fps, visits)
		}
		if expected == nil {
			expected = visits
		} else if !reflect.DeepEqual(expected, visits) {
			t.Fatal("presentation cadence changed source shake sequence", fps, visits)
		}
		t.Logf("native clock fps%d settled after%d logical visits /%.3fs sampled host time; original DOS wall time not asserted", fps, ticks, float64(frames)/float64(fps))
	}
}
