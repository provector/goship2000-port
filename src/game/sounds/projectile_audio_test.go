package sounds

import (
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

// The source application never sends a projectile ID to ASOUND. Event17 is a
// row6 stream replacement, not a finite sound attached to a combat slot. Impact
// row0 cannot retire it; a reached0bh replaces it on that same retained row6.
func TestProjectileViewSourceSustainSurvivesImpact(t *testing.T) {
	commands, source := combatCommandFixture(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(10); err != nil {
		t.Fatal(err)
	}
	if err = q.gs2FixedCommand(commands[0x17]); err != nil {
		t.Fatal(err)
	}
	if err = q.gs2FixedCommand(commands[0x10]); err != nil {
		t.Fatal(err)
	}
	for n := 0; len(q.loops[6]) < 3 && n < 30000; n++ {
		q.interrupt()
	}
	if q.err != nil || len(q.loops[6]) < 3 || q.mem[q.base(6)] == 0 || q.word(l.NoiseKind) != 0 {
		t.Fatal("source projectile sustain was made finite by impact", q.err, q.loops[6])
	}
	t.Logf("source row6 loop PIT boundaries=%v; no projectile identity/slot consumer in audio driver", q.loops[6])
	if q.loops[6][2]-q.loops[6][1] != q.loops[6][1]-q.loops[6][0] {
		t.Fatal("not stable original alternate loop", q.loops[6])
	}
	if err = q.rotorCommand(10); err != nil {
		t.Fatal(err)
	}
	if q.word(l.NoiseKind) != 1 {
		t.Fatal("0b did not resume retained rotor")
	}
}

func TestProjectileSoundTransportPauseMuteAndExit(t *testing.T) {
	commands, _ := combatCommandFixture(t)
	p, err := NewAudioPlayer(&assetdata.AudioAssets{GS2Commands: commands}, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	r := &combatFakeRotor{}
	p.rotor = r
	if err = p.PlayGS2Event(0x17, 0); err != nil {
		t.Fatal(err)
	}
	p.SetEffectsPaused(true)
	if !r.paused {
		t.Fatal("Stop did not pause retained row")
	}
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(false)
	if err = p.PlayGS2Event(0x10, 0); err != nil {
		t.Fatal(err)
	}
	p.SetSoundEffectsEnabled(true)
	if r.closes != 0 || len(r.fixed) != 2 || r.fixed[0] != 0x17 || r.fixed[1] != 0x10 {
		t.Fatal("transport restarted/dropped source row while muted", r)
	}
	p.UpdateRotor(10)
	if len(r.commands) != 1 || r.commands[0] != 10 {
		t.Fatal("missing source0b restoration", r)
	}
	p.StopRotor() // Library exit/Rewind owns a deliberate whole-session reset.
	p.StopRotor()
	if r.closes != 1 || p.rotor != nil {
		t.Fatal("exit retained or double closed worker", r)
	}
}
