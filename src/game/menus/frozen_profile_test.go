package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"testing"
)

func TestRecordedAutonomyIgnoresCurrentSetupAndPracticeCannotCommit(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint("disabled=", disabled), func(t *testing.T) {
			c := renderingController(t)
			c.Runtime.DisableWingmenAI = disabled
			startRendering(t, c)
			jFinishFade(t, c)
			p := c.Rendering
			if !p.r1Mechanics || p.wingmenAutonomy == disabled {
				t.Fatal("launch did not freeze new effective setting")
			}
			c.Handle(Actions{})
			c.Handle(Actions{FlightCommands: []int{2}})
			for i := 0; i < 10; i++ {
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
			}
			// A later mutable SETUP-like value cannot alter this running sortie.
			c.Runtime.DisableWingmenAI = !disabled
			if p.wingmenAutonomy == disabled {
				t.Fatal("running policy changed")
			}
			endFilm(t, c)
			f, err := c.SealRecordedFilm()
			if err != nil {
				t.Fatal(err)
			}
			if !f.R1Mechanics() || f.Header.InputSchema != replay.R1InputCompatibility {
				t.Fatal("new interpretation missing")
			}
			before, _ := json.Marshal(c.Store.State())
			filmBefore, _ := replay.Marshal(f)
			opt := configuration.Defaults()
			opt.DisableWingmenAI = !disabled
			q, err := NewFilmPlayback(c.Office.Assets, f, opt)
			if err != nil {
				t.Fatal(err)
			}
			if q.Preview().wingmenAutonomy == disabled || q.Engine.Runtime.DisableWingmenAI != disabled {
				t.Fatal("playback read current setting")
			}
			for !q.Done {
				if err = q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			practice, err := NewFilmPlayback(c.Office.Assets, f, opt)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err = practice.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if err = practice.Resume(); err != nil {
				t.Fatal(err)
			}
			if practice.Preview().wingmenAutonomy == disabled || practice.Engine.Store != nil || practice.Engine.Mission.ResultLaunch != nil {
				t.Fatal("practice changed frozen policy or acquired career")
			}
			practice.HandlePractice(Actions{})
			practice.AdvancePractice(.1)
			practice.HandlePractice(Actions{EndMission: true})
			practice.HandlePractice(Actions{ConfirmQuit: true})
			after, _ := json.Marshal(c.Store.State())
			filmAfter, _ := replay.Marshal(f)
			if !bytes.Equal(before, after) || !bytes.Equal(filmBefore, filmAfter) {
				t.Fatal("playback/practice mutated owner")
			}
		})
	}
}
