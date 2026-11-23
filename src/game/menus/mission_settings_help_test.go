package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestSettingsHelpUsesArea12Font2CurrentChoiceAndSourceColours(t *testing.T) {
	help := make([]assetdata.SettingsHelp, 9)
	for i := range help {
		help[i] = assetdata.SettingsHelp{Key: i, Heading: "H", Body: "B", Choices: []string{"A", "ZZ"}}
	}
	help[8].Choices = nil // This generic-only synthetic case intentionally has no APU state text.
	a := &assetdata.MissionAssets{Fonts: map[int]*assetdata.BitmapFont{2: briefingTestFont()}, Preflight: &assetdata.PreflightAssets{SettingsHelp: help}}
	frame := bytes.Repeat([]byte{77}, 64000)
	settings := gamestate.PreflightSettings{CPGControls: 1}
	drawSettingsHelp(frame, a, settings, 1)
	// Independent geometry/color operands: AREA12 200,8,100,56; 29h/24h/29h.
	if frame[8*320+249] != 41 || frame[15*320+200] != 36 || frame[22*320+200] != 41 || frame[22*320+201] != 41 {
		t.Fatal("heading/body/current-choice or source colors not drawn")
	}
	if frame[22*320+202] != 0 || frame[8*320+199] != 77 || frame[64*320+200] != 77 {
		t.Fatal("MFD drawing escaped AREA12 or ignored the committed value")
	}
	if settings.CPGControls != 1 || help[0].Choices[1] != "ZZ" {
		t.Fatal("help drawing changed settings or prepared text")
	}
	for _, hit := range []int{0, -1, 10, 11} {
		blank := bytes.Repeat([]byte{77}, 64000)
		drawSettingsHelp(blank, a, settings, hit)
		if !bytes.Equal(blank, bytes.Repeat([]byte{77}, 64000)) {
			t.Fatal("unrecognized/knob hotspot drew invented help", hit)
		}
	}
	frame = bytes.Repeat([]byte{77}, 64000)
	drawSettingsHelp(frame, a, settings, 8)
	if frame[8*320+249] != 41 || frame[15*320+200] != 0 {
		t.Fatal("key7 must omit the body even when supplied one")
	}
	frame = bytes.Repeat([]byte{77}, 64000)
	drawSettingsHelp(frame, a, settings, 9)
	if frame[8*320+249] != 41 || frame[15*320+200] != 36 || frame[22*320+200] != 0 {
		t.Fatal("APU must draw generic explanation without a choice")
	}
}
