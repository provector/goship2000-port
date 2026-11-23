package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func finishPaperMotion(t *testing.T, c *FrontEnd) {
	t.Helper()
	for i := 0; c.Screen == ScreenPaperTransition && i < 100; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen == ScreenPaperTransition {
		t.Fatal("paper transition did not finish")
	}
}

func TestPaperScrollClipsAndReachesExactEndpoints(t *testing.T) {
	from, to := make([]byte, 64000), make([]byte, 64000)
	for i := range from {
		from[i] = 1
		to[i] = 2
	}
	for _, kind := range []paperMotion{paperOpening, paperNext, paperClosing} {
		motion := &paperTransition{Motion: kind, From: from, To: to}
		if !bytes.Equal(motion.frame(), from) {
			t.Fatal("transition did not start at its source")
		}
		motion.Offset = motion.extent() / 2
		f := motion.frame()
		if kind == paperNext && (f[227] != 1 || f[226] != 2 || f[0] != 1) {
			t.Fatal("horizontal panel clipping/direction")
		}
		if kind == paperOpening && (f[0] != 1 || f[190*320] != 2) {
			t.Fatal("opening must move up")
		}
		if kind == paperOpening && (f[85*320] != 2 || f[85*320+250] != 1) {
			t.Fatal("caption lead must join the sheet before right artwork")
		}
		if kind == paperClosing && (f[0] != 2 || f[190*320] != 1) {
			t.Fatal("closing must move down")
		}
		motion.Offset = motion.extent()
		f = motion.frame()
		for y := 0; y < 191; y++ {
			for x := 0; x < 320; x++ {
				want := byte(2)
				if kind == paperNext && x >= 227 {
					want = 1
				}
				if f[y*320+x] != want {
					t.Fatal("final transition pixels differ")
				}
			}
		}
		if f[191*320] != 1 {
			t.Fatal("footer scrolled with the page")
		}
	}
}

func TestPaperVerticalHandMovesAsOneClippedSprite(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	c.beginPaperTransition(paperOpening)
	c.Mission.PaperTransition.Offset = 168
	check := func(offset int) {
		t.Helper()
		f := c.Frame(false)
		src := c.Mission.View.Assets.Images["HANDS.PIC"]
		count := 0
		for y := 0; y < min(57, 191-134-offset); y++ {
			for x := 0; x < 71; x++ {
				colour := src[y*320+249+x]
				if colour != 0 {
					count++
					if f[(134+offset+y)*320+199+x] != colour {
						t.Fatal("vertical hand split across panel/right artwork")
					}
				}
			}
		}
		if count == 0 {
			t.Fatal("empty moving-hand witness")
		}
	}
	check(23)
	finishPaperMotion(t, c)
	c.beginPaperTransition(paperClosing)
	c.Mission.PaperTransition.Offset = 24
	check(24)
}

func TestPaperTransitionsReuseMissionFreezeContentAndResumeQuit(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	before, _ := json.Marshal(c.Mission.Data)
	saved := c.Store.State()
	session := c.Mission
	output := os.Getenv("GS2000_D1_FRAME_OUTPUT")
	if output != "" {
		if err := os.Mkdir(output, 0755); err != nil {
			t.Fatal(err)
		}
	}
	watch := func(name string) {
		t.Helper()
		if c.Screen != ScreenPaperTransition {
			t.Fatal("missing transition", name)
		}
		for i := 0; i < 6; i++ {
			if err := c.Tick(); err != nil {
				t.Fatal(err)
			}
		}
		f := c.Frame(false)
		if !bytes.Equal(f, c.Frame(false)) {
			t.Fatal("drawing rerendered/changed cached page")
		}
		if output != "" {
			if err := WriteFrontEndFrame(filepath.Join(output, name+".png"), c, false); err != nil {
				t.Fatal(err)
			}
		}
		old := c.Mission.PaperTransition
		act(t, c, Actions{Selector1: true, Selector2: true, Cancel: true, Inside: true, X: 1, Y: 1})
		if c.Mission.PaperTransition != old || c.Screen != ScreenPaperTransition {
			t.Fatal("input interrupted transition")
		}
	}
	x, y, _ := missionActionPoint(c, 2)
	act(t, c, Actions{Selector1: true, Inside: true, X: x, Y: y})
	watch("opening")
	motion := session.PaperTransition
	offset := motion.Offset
	frame := c.Frame(false)
	act(t, c, Actions{Quit: true})
	for i := 0; i < 20; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if motion.Offset != offset {
		t.Fatal("quit advanced transition")
	}
	act(t, c, Actions{Text: []rune{'n'}})
	if c.Screen != ScreenPaperTransition || session.PaperTransition != motion || !bytes.Equal(frame, c.Frame(false)) {
		t.Fatal("quit cancel lost motion")
	}
	finishPaperMotion(t, c)
	for _, page := range []int{1, 2, 3, 0} {
		x, y, _ = missionActionPoint(c, 3)
		act(t, c, Actions{Selector1: true, Inside: true, X: x, Y: y})
		if session.Page != page {
			t.Fatal("page/wrap", page, session.Page)
		}
		watch(fmt.Sprintf("next-%d", page))
		if page == 1 {
			f := c.Frame(false)
			draw := session.View.visibilityDraw
			session.View.visibilityDraw ^= 1
			if !bytes.Equal(f, c.Frame(false)) {
				t.Fatal("Intel page was rendered again during scroll")
			}
			session.View.visibilityDraw = draw
		}
		// Foreground hand pixels remain at their original coordinates while the
		// sheet scrolls underneath; source zero pixels remain transparent.
		f := c.Frame(false)
		hand := session.View.Assets.Images["HANDS.PIC"]
		count := 0
		for yy := 0; yy < 57; yy++ {
			for xx := 0; xx < 71; xx++ {
				if colour := hand[yy*320+249+xx]; colour != 0 {
					count++
					if f[(134+yy)*320+199+xx] != colour {
						t.Fatal("hand moved with prose")
					}
				}
			}
		}
		if count == 0 {
			t.Fatal("empty source hand witness")
		}
		finishPaperMotion(t, c)
		if c.Screen != ScreenPapers {
			t.Fatal(c.Screen)
		}
	}
	act(t, c, Actions{Cancel: true})
	watch("closing")
	finishPaperMotion(t, c)
	if c.Screen != ScreenBriefing || c.Mission != session || session.Page != 0 {
		t.Fatal("wrong return/session")
	}
	after, _ := json.Marshal(session.Data)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(saved, c.Store.State()) {
		t.Fatal("transitions changed mission/save")
	}
}

func TestPaperTransitionConsumesHeldOpeningInput(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	x, y, _ := missionActionPoint(c, 2)
	latch := inputLatch{}
	for i := 0; i < 40; i++ {
		act(t, c, latch.Read(Buttons{X: x, Y: y, Inside: true, Space: true}))
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenPapers || c.Mission.Page != 0 {
		t.Fatal("held opening input skipped a page")
	}
	act(t, c, latch.Read(Buttons{X: x, Y: y, Inside: true}))
	act(t, c, latch.Read(Buttons{X: x, Y: y, Inside: true, Space: true}))
	if c.Screen != ScreenPaperTransition || c.Mission.Page != 1 {
		t.Fatal("fresh press failed after release")
	}
}
