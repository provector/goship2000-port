package menus

import (
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/sounds"
	"gunship2000-assets/game/state"
	"testing"
)

func TestMainCameraWingmanAudio(t *testing.T) {
	m := &state.InitializedMission{PlayerRow: 0, FlightRows: []int{0, 1, 2}, Entities: []state.InitializedEntity{{ID: "pilot"}, {ID: "one"}, {ID: "two"}, {ID: "support", Kind: 0x11}, {ID: "artillery", Kind: 0x19}}}
	r := &FlightRuntime{Combat: &combat.State{}, Avionics: &avionics.State{}}
	v := &RenderingPreview{Initialized: m, Flight: r, ViewControls: &render3d.CameraController{}}
	c := &FrontEnd{Screen: ScreenScene, Rendering: v}
	a := &assets.AudioAssets{GS2Commands: map[uint16]assets.AudioGS2Command{}, Clips: map[string]*assets.AudioClip{}}
	for event, key := range map[uint16]string{7: "gun", 8: "launch", 0x10: "impact"} {
		a.GS2Commands[event] = assets.AudioGS2Command{EventID: event, TrackKey: key}
		a.Clips[key] = &assets.AudioClip{Samples: []int16{int16(event * 128), int16(event * 128), int16(event * 128)}, SampleRate: 49716}
	}
	ap, err := sounds.NewAudioPlayer(a, sounds.DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Stop()
	r.Combat.Slots[16].Shooter = 1
	r.Combat.Slots[17].Shooter = 2
	emit := func(event uint16, slot int) {
		r.Combat.Events = append(r.Combat.Events, combat.Event{Kind: "sound", Actor: 0, Slot: slot, Command: event})
	}
	check := func(want uint16) {
		t.Helper()
		v.collectCombatEvents()
		syncFrontEndAudio(ap, c)
		var dst [1]float32
		ap.Render(dst[:], 1)
		if dst[0] != float32(want)/256 {
			t.Fatalf("camera %d %q: got %f want %f", v.ViewControls.Mode, v.ViewControls.TrackedID, dst[0], float32(want)/256)
		}
	}
	r.PrimaryMFD = 5
	r.Avionics.Target = 2
	emit(7, 16)
	emit(8, 17)
	check(0)
	v.ViewControls.Mode = 4
	v.ViewControls.TrackedID = "one"
	emit(7, 16)
	emit(8, 17)
	check(7)
	v.ViewControls.TrackedID = "two"
	check(0)
	emit(8, 17)
	emit(7, 16)
	check(8)
	v.ViewControls.Mode = 0
	emit(0x10, 17)
	check(0x10)
	emit(7, 0)
	check(7)
	if len(r.CombatEvents) != 8 {
		t.Fatalf("canonical combat events: %d", len(r.CombatEvents))
	}

	// Air support uses the same low projectile slot pool as player weapons.
	// A simultaneous artillery or player cue is still audible in the cockpit.
	r.Combat.Slots[3].Shooter = 3
	r.Combat.Slots[4].Shooter = 4
	ap.StopEffects()
	emit(8, 3)
	check(0)
	emit(7, 4)
	check(7)
	ap.StopEffects()
	v.ViewControls.Mode, v.ViewControls.TrackedID = 10, "support"
	emit(7, 16)
	emit(8, 3)
	check(8)
	v.GPS = &navigation.GPS{}
	emit(7, 3)
	check(0)
	v.GPS = nil
	v.ViewControls.Mode = 0
	check(0)
	emit(0x10, 3)
	check(0x10)

	// A practice cockpit owns the main camera even if the VCR had been
	// following a wingman immediately before Resume.
	ap.StopEffects()
	viewer := &ReplayViewer{Actor: 1, Playback: &FilmPlayback{Engine: c}}
	c = &FrontEnd{Screen: ScreenReplay, Viewer: viewer}
	emit(7, 16)
	check(7)
	viewer.Practice = true
	emit(7, 16)
	check(0)
	v.ViewControls.Mode = 4
	v.ViewControls.TrackedID = "two"
	emit(8, 17)
	check(8)
}
