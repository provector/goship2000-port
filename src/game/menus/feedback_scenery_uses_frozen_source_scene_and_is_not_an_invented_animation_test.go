package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func sceneCapture(t *testing.T, c *FrontEnd, label string, frame []byte) {
	t.Helper()
	dir := os.Getenv("GS2000_P6_SCENE_FRAMES")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		img := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), c.debriefPalette())
		for y := 0; y < 200*scale; y++ {
			for x := 0; x < 320*scale; x++ {
				img.Pix[y*img.Stride+x] = frame[(y/scale)*320+x/scale]
			}
		}
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s-ui%dx.png", label, scale)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestFeedbackSceneryUsesFrozenSourceSceneAndIsNotAnInventedAnimation(t *testing.T) {
	c := debriefDisplay(t)
	v := c.Debrief
	p := v.Pending.Proposal
	for _, scene := range []string{"HAPPY.SCN", "SAD.SCN"} {
		v.Page, v.Scene = "feedback", scene
		for kind := byte(0); kind <= 5; kind++ {
			p.Facts.ResultContext.Version = 3
			p.Facts.ResultContext.Scene = &kind
			v.Tick = 0
			before, _ := json.Marshal(p)
			frame := c.debriefFrame()
			sceneCapture(t, c, fmt.Sprintf("%s-scene%d-native-t000", scene, kind), frame)
			background, _, ok := c.frozenFeedbackMedia()
			if !ok {
				t.Fatal("frozen scene missing")
			}
			want := append([]byte(nil), c.Office.Assets.Debrief.Images[background]...)
			definition := c.Office.Assets.Debrief.Scenes[scene]
			r := definition.Areas[1].Rectangle
			if kind < 4 {
				if err := c.Office.Assets.Debrief.Lori(want, scene, 0, int(kind), r[0], r[1]); err != nil {
					t.Fatal(err)
				}
			}
			for y := 0; y < 191; y++ {
				if !bytes.Equal(frame[y*320:(y+1)*320], want[y*320:(y+1)*320]) {
					t.Fatalf("%s frozenScene%d row%d wrong scenery/palette context", scene, kind, y)
				}
			}
			for _, tick := range []int{1, 5, 49, 119} {
				v.Tick = tick
				if !bytes.Equal(c.debriefFrame(), frame) {
					t.Fatalf("source-static feedback background invented motion %s/%d tick%d", scene, kind, tick)
				}
			}
			sceneCapture(t, c, fmt.Sprintf("%s-scene%d-native-t119", scene, kind), c.debriefFrame())
			after, _ := json.Marshal(p)
			if !bytes.Equal(before, after) {
				t.Fatal("drawing changed result facts")
			}
		}
	}
	// The original CREWPRO resource has neither a background nor ANIM records;
	// its four dynamic data panels are distinct from the crew-chief feedback art.
	crew := c.Office.Assets.Debrief.Scenes["CREWPRO.SCN"]
	if crew.Background != "" || len(crew.Sprites) != 0 || len(crew.Components) != 0 {
		t.Fatal("unexpected crew background data", crew)
	}
	c.enterDebriefPage("crew")
	base := c.crewDebriefFrame()
	c.Debrief.Tick = 10
	if !bytes.Equal(base, c.crewDebriefFrame()) {
		t.Fatal("crew panels acquired an unsupported animated background")
	}
	// Real existing motion belongs to the separate source pointer, not an
	// invented room/background loop. Compare received logical ticks and output.
	c.Office.SetPointer(3, 146, true)
	c.Debrief.Tick = 0
	one := c.crewDebriefFrame()
	c.drawCrewPointer(one)
	c.Debrief.Tick = 5
	two := c.crewDebriefFrame()
	c.drawCrewPointer(two)
	if bytes.Equal(one, two) || !bytes.Equal(one[:137*320], two[:137*320]) {
		t.Fatal("crew pointer motion/background ownership differs")
	}
	sceneCapture(t, c, "CREWPRO-native-pointer-t000", one)
	sceneCapture(t, c, "CREWPRO-native-pointer-t005", two)
	p.Facts.ResultContext = &gamestate.SortieResultContext{}
	if selector, enabled := c.feedbackScenerySelector(); selector != 2 || !enabled {
		t.Fatal("historical no-context presentation changed")
	}
}
