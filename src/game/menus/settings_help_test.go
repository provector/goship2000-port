package menus

import (
	"bytes"
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

func TestSettingsFeedbackSourceLatchOrdering(t *testing.T) {
	s := SettingsHelpState{}
	s.Observe(-1, 2, false)
	if s.Detailed {
		t.Fatal("first hover must be generic")
	}
	s.Observe(2, 2, true)
	if !s.Detailed {
		t.Fatal("same-area activation must select detailed feedback")
	}
	s.Observe(2, 2, false)
	if !s.Detailed {
		t.Fatal("stationary hover erased feedback")
	}
	s.Observe(2, 2, true)
	if !s.Detailed {
		t.Fatal("repeated activation lost feedback")
	}
	s.Observe(2, 5, true)
	if s.Detailed {
		t.Fatal("source area-change clear must follow activation's pending flag")
	}
	for i := 0; i < 600; i++ {
		s.Tick()
	}
	if s.Detailed {
		t.Fatal("source uses >600, not >=600")
	}
	s.Tick()
	if !s.Detailed {
		t.Fatal("idle callback producer did not request detailed refresh")
	}
	s.Observe(5, -1, false)
	if s.Detailed || s.idleTicks != 0 {
		t.Fatal("leaving controls did not reset feedback")
	}
}

func TestSettingsDetailedTextReplacesGenericWithSourceInk(t *testing.T) {
	help := make([]assetdata.SettingsHelp, 9)
	for key := range help {
		help[key] = assetdata.SettingsHelp{Key: key, Heading: "H", Body: "B", Choices: []string{"A", "ZZ"}, Details: []string{"D", "EEE"}}
	}
	help[8].Choices = nil
	a := &assetdata.MissionAssets{Fonts: map[int]*assetdata.BitmapFont{2: briefingTestFont()}, Preflight: &assetdata.PreflightAssets{SettingsHelp: help}}
	frame := bytes.Repeat([]byte{77}, 64000)
	settings := gamestate.PreflightSettings{CPGControls: 1}
	state := SettingsHelpState{Detailed: true}
	drawSettingsHelpMode(frame, a, settings, 1, state, 0)
	// Hand/source operands: heading29h, then detailed4c3a text in29h after
	// one newline; generic24h body and another selected-value row do not follow.
	if frame[8*320+249] != 41 || frame[15*320+200] != 41 || frame[15*320+202] != 41 || frame[22*320+200] != 0 {
		t.Fatal("detailed source branch combined stale generic/value rows")
	}
	if state != (SettingsHelpState{Detailed: true}) || settings.CPGControls != 1 {
		t.Fatal("drawing mutated transient or saved state")
	}
	for _, hit := range []int{8, 9} {
		f := bytes.Repeat([]byte{77}, 64000)
		drawSettingsHelpMode(f, a, settings, hit, state, 0)
		if hit == 8 && f[15*320+200] != 0 {
			t.Fatal("key7 body must remain omitted")
		}
		if hit == 9 && (f[15*320+200] != 36 || f[22*320+200] != 0) {
			t.Fatal("key8/APU must stay generic without choice")
		}
	}
	// Every supported current value selects its own prepared detailed text. The
	// synthetic one-pixel font makes widths independently observable.
	for key, count := range []int{3, 4, 2, 2, 2, 2, 2} {
		a.Preflight.SettingsHelp[key].Details = nil
		for value := 0; value < count; value++ {
			a.Preflight.SettingsHelp[key].Details = append(a.Preflight.SettingsHelp[key].Details, string(bytes.Repeat([]byte{'X'}, value+1)))
		}
		for value := 0; value < count; value++ {
			settings.Set(key, value)
			f := bytes.Repeat([]byte{77}, 64000)
			drawSettingsHelpMode(f, a, settings, key+1, state, 0)
			for x := 0; x < count+1; x++ {
				want := byte(0)
				if x <= value {
					want = 41
				}
				if f[15*320+200+x] != want {
					t.Fatalf("key%d value%d width%d", key, value, x)
				}
			}
		}
	}
}

func TestSettingsAPUShortStateUsesActualPhaseAndGenericBody(t *testing.T) {
	help := make([]assetdata.SettingsHelp, 9)
	help[8] = assetdata.SettingsHelp{Key: 8, Heading: "H", Body: "B", Choices: []string{"SAFETY ON", "SAFETY OFF", "POWER ON"}}
	a := &assetdata.MissionAssets{Fonts: map[int]*assetdata.BitmapFont{2: briefingTestFont()}, Preflight: &assetdata.PreflightAssets{SettingsHelp: help}}
	for phase, text := range help[8].Choices {
		frame := bytes.Repeat([]byte{77}, 64000)
		drawSettingsHelpMode(frame, a, gamestate.PreflightSettings{}, 9, SettingsHelpState{Detailed: true}, byte(phase))
		if frame[15*320+200] != 36 {
			t.Fatal("APU detailed latch must retain generic body")
		}
		for x := 0; x < 11; x++ {
			want := byte(0)
			if x < len(text) {
				want = 41
			}
			if frame[22*320+200+x] != want {
				t.Fatalf("APUphase%d selected text width at%d", phase, x)
			}
		}
	}
}

func TestSettingsHelpSourceSubstringWrappingAndCentering(t *testing.T) {
	f := briefingTestFont()
	for _, c := range []struct {
		text  string
		width int
		want  []string
	}{
		{"POOR:  The bottom", 10, []string{"POOR:  The ", "bottom"}},
		{"one-two-three", 6, []string{"one-", "two-", "three"}},
		// C05ec records a delimiter before checking its width. An overflowing
		// hyphen remains in the copied substring and is clipped by AREA12.
		{"one-two-three", 7, []string{"one-two-", "three"}},
		{"a\nb", 20, []string{"a", "b"}},
		{"ABCDE", 3, []string{"ABC", "DE"}},
	} {
		if got := settingsHelpLines(f, c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%q: %q want%q", c.text, got, c.want)
		}
	}
	help := make([]assetdata.SettingsHelp, 9)
	help[1] = assetdata.SettingsHelp{Key: 1, Heading: "ENEMY QUALITY:", Body: "B", Choices: []string{"A"}}
	a := &assetdata.MissionAssets{Fonts: map[int]*assetdata.BitmapFont{2: f}, Preflight: &assetdata.PreflightAssets{SettingsHelp: help}}
	frame := make([]byte, 64000)
	drawSettingsHelp(frame, a, gamestate.PreflightSettings{}, 2)
	// Literal C0578 operands: left+(width-measured heading)/2, truncating.
	x := 200 + (100-len("ENEMY QUALITY:"))/2
	if frame[8*320+x] != 41 || frame[8*320+x-1] != 0 || frame[8*320+200] != 0 {
		t.Fatal("heading did not use AREA-centered style1")
	}
}
