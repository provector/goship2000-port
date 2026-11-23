package sounds

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func initializationSource(t *testing.T) []byte {
	t.Helper()
	p := &assetdata.IslandIce469085
	source, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), p.Source(assetdata.RoleAudioADigital).Name))
	if os.IsNotExist(err) {
		t.Skip("optional original initialization driver unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if assetdata.Hash(source) != p.Source(assetdata.RoleAudioADigital).SHA256 {
		t.Fatal("unsupported original identity")
	}
	return source
}

func TestInitializationAudioOriginalInstructionsAndFirstNotes(t *testing.T) {
	source := initializationSource(t)
	check := func(at int, expected string) {
		t.Helper()
		b, err := hex.DecodeString(expected)
		if err != nil || !bytes.Equal(source[at:at+len(b)], b) {
			t.Fatalf("original instruction differs at%04x", at)
		}
	}
	// Module-local CS, MZ header0200. GS2 is not the GS4 table/domain.
	check(0x200+0x112f, "8b5e0683fb4a7f101eb849028ed8d1e333c02eff974f10")
	check(0x200+0x941, "833e920c017426c706920c0100")
	check(0x200+0x96e, "8b5e088bc3d1e8053200a2980c")
	check(0x200+0x4b1, "833e920c00745c803e400e007555")
	check(0x200+0x4ce, "781183fb15761abb1500c706960c0000")
	check(0x200+0x5e6, "b8904c")
	check(0x200+0x6c2, "b8880f")
	if binary.LittleEndian.Uint16(source[0x200+0x104f+0x0b*2:]) != 0x941 || binary.LittleEndian.Uint16(source[0x200+0x104f+0x2b*2:]) != 0xa9a || source[0x200+0xa9a] != 0xc3 {
		t.Fatal("original constructor/no-op mapping")
	}
	// Independent loader operands bind command/argument to the actual entry path.
	gs, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if assetdata.Hash(gs) != assetdata.IslandIce469085.Source(assetdata.RoleSimulationExecutable).SHA256 {
		t.Fatal("unsupported GS2")
	}
	call, _ := hex.DecodeString("6a016a0b9a0b00440883c4046a2b9a0b004408")
	if !bytes.Equal(gs[0xa120+0x1459:0xa120+0x1459+len(call)], call) {
		t.Fatal("original initialization audio call")
	}
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, err := newAudioSequence(source, l, assetdata.IslandIce469085.AudioLayout)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.startInitializationChannel(); err != nil {
		t.Fatal(err)
	}
	q.interrupt()
	b := q.base(6)
	if q.err != nil || q.mem[b] != 30 || q.mem[b+4] != 41 || q.mem[b+5] != 23 || q.word(l.NoiseSpeed) != 0 {
		t.Fatal("first source note/instrument/duration", q.err)
	}
	// Note41 => remainder5 => source FNUM02AB, block3. Source note/key-on
	// writes A6=AB, B6=0E then2E. These expectations are hand-derived.
	var tail []audioTimedWrite
	for _, w := range q.writes {
		if w.Register == 0xa6 || w.Register == 0xb6 {
			tail = append(tail, w)
		}
	}
	n := len(tail)
	if n < 3 || tail[n-3].Register != 0xa6 || tail[n-3].Value != 0xab || tail[n-2].Register != 0xb6 || tail[n-2].Value != 0x0e || tail[n-1].Value != 0x2e {
		t.Fatalf("hand-worked first pitch/key writes:%v", tail)
	}
	for i := 1; i < 31; i++ {
		q.interrupt()
	}
	if q.mem[b] != 0 || q.word(l.NoiseSpeed) != 0 {
		t.Fatal("source end pair before refresh")
	}
	q.interrupt()
	if q.mem[b] != 22 || q.word(l.NoiseSpeed) != 1 || q.mem[b+5] != 23 {
		t.Fatal("second source speed/duration/instrument")
	}
}

func TestInitializationAudioRampBoundaryAndDeterminism(t *testing.T) {
	source := initializationSource(t)
	layout := assetdata.IslandIce469085.AudioLayout
	l := layout.Drivers["GS2"]
	a, err := initializationSoundSequence(source, l, layout)
	if err != nil {
		t.Fatal(err)
	}
	b, err := initializationSoundSequence(source, l, layout)
	if err != nil {
		t.Fatal(err)
	}
	// Raw source duration lookup at0c99. Each pair is followed by00/00 and
	// one refresh tick. Stop occurs after first full speed21 note ends.
	durations := []int{30, 22, 21, 19, 17, 15, 14, 13, 12, 11, 10, 10, 9, 9, 8, 8, 8, 7, 7, 7, 7, 6}
	ticks := 1
	for _, duration := range durations {
		ticks += duration + 1
	}
	if a.ticks != uint64(ticks) || a.pit != uint64(ticks*0x4c90) || a.busy() != 0 || a.word(l.NoiseKind) != 0 {
		t.Fatalf("bounded ramp timing ticks%d/%d PIT%d busy%d", a.ticks, ticks, a.pit, a.busy())
	}
	if len(a.writes) != len(b.writes) {
		t.Fatal("nondeterministic ramp")
	}
	for i, w := range a.writes {
		if w != b.writes[i] {
			t.Fatal("nondeterministic write", i)
		}
	}
}

func TestInitializationAudioEffectPreferencesAndOwner(t *testing.T) {
	clip := &assetdata.AudioClip{Samples: []int16{1000, 2000, 3000}, SampleRate: 49716}
	p, err := NewAudioPlayer(&assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{InitializationStartClip: clip}}, AudioPreferences{MusicMode: MusicAll, MusicVolume: 1, EffectsVolume: .25})
	if err != nil {
		t.Fatal(err)
	}
	p.Sync("silent", nil, false)
	p.PlayInitializationStart()
	if s := p.Status(); s.InitializationCommands != 1 || s.LastCue != 0 || s.MusicOwner != "silent" {
		t.Fatal(s)
	}
	data := make([]float32, 1)
	p.Render(data, 1)
	if data[0] != 250.0/32768 {
		t.Fatal("independent effect gain", data)
	}
	p.SetSoundEffectsEnabled(false)
	p.PlayInitializationStart()
	p.Render(data, 1)
	if data[0] != 0 || p.Status().InitializationCommands != 1 {
		t.Fatal("effects mute must stop/suppress start")
	}
	p.SetSoundEffectsEnabled(true)
	p.Render(data, 1)
	if data[0] != 0 {
		t.Fatal("unmute replayed consumed initialization")
	}
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	if p.Status().MusicOwner != "silent" {
		t.Fatal("music toggle manufactured a scene owner")
	}
	p.PlayInitializationStart()
	if p.Status().InitializationCommands != 2 {
		t.Fatal("future initialization must remain available")
	}
	p.Sync("silent", nil, true)
	p.PlayInitializationStart()
	if p.Status().InitializationCommands != 2 {
		t.Fatal("quit restarted effect")
	}
}
