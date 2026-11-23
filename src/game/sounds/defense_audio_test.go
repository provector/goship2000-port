package sounds

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func TestSuccessfulDefenseOriginalCallersAndFixedConstructors(t *testing.T) {
	g, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, expected := range map[int]string{
		0x106a: "803ede00007503e901f76a2ae90ffb", // mainDE gate after successful flare mutation
		0x1174: "803ede00007503e9f7f56a0ae905fa", // chaff, not the similarly numbered GS4 cue
		0x0b88: "9a0b004408e912fd",               // shared selected-driver veneer, then common input tail
	} {
		want, _ := hex.DecodeString(expected)
		if !bytes.Equal(g[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source caller %04x", ip)
		}
	}
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(15); err != nil {
		t.Fatal(err)
	}
	rotor := append([]byte(nil), q.mem[q.base(6):q.base(6)+30]...)
	for _, tc := range []struct {
		id, handler, start, row int
		raw                     string
	}{
		{0x0a, 0x093a, 0x0924, 2, "8d0e2409e9cffd"},
		{0x2a, 0x0933, 0x099a, 0, "8d0e9a09e9c8fd"},
	} {
		want, _ := hex.DecodeString(tc.raw)
		if int(audioWord(source, 512+l.EventTableCS+2*tc.id)) != tc.handler || !bytes.Equal(source[512+tc.handler:][:7], want) {
			t.Fatal("actual ASOUND table/wrapper", tc)
		}
		c, er := prepareGS2Command(source, l, assetdata.IslandIce469085.AudioLayout, tc.id)
		if er != nil || c.Channel != tc.row || c.Start != tc.start || c.Alternate != 0 || c.HandlerHex != tc.raw || c.ClearNoise {
			t.Fatal(c, er)
		}
		if er = q.gs2FixedCommand(c); er != nil {
			t.Fatal(er)
		}
		if !bytes.Equal(rotor, q.mem[q.base(6):q.base(6)+30]) {
			t.Fatal("defense replaced rotor")
		}
		// Repeated successful manual deployments reset this exact row, not every
		// row or private clock/RNG. Source has no additional cooldown gate.
		pit, rng := q.pit, q.word(l.RNG)
		if er = q.gs2FixedCommand(c); er != nil || q.word(q.base(tc.row)+14) != uint16(tc.start) || q.pit != pit || q.word(l.RNG) != rng {
			t.Fatal(er)
		}
	}
}

func TestDefenseSoundSharedRowPriorityMutePauseAndFiniteLifetime(t *testing.T) {
	commands, source := combatCommandFixture(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	q.rotorCommand(10)
	// Flare shares row0 with actual impact/destruction; source-later wins.
	for _, id := range []uint16{0x2a, 0x10, 0x0a, 0x14, 0x09} {
		if err = q.gs2FixedCommand(commands[id]); err != nil {
			t.Fatal(err)
		}
	}
	for row, want := range map[int]uint16{0: 0x948, 1: 0x91c, 2: 0x924, 4: 0x9a2} {
		if q.word(q.base(row)+14) != want {
			t.Fatalf("row%d", row)
		}
	}
	for i := 0; i < 10000 && (q.mem[q.base(0)] != 0 || q.mem[q.base(2)] != 0); i++ {
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
	}
	if q.mem[q.base(0)] != 0 || q.mem[q.base(2)] != 0 || q.word(l.NoiseKind) != 1 {
		t.Fatal("finite defense stream/retained rotor lifetime")
	}
	a := &assetdata.AudioAssets{GS2Commands: commands, Clips: map[string]*assetdata.AudioClip{}}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	r := &combatFakeRotor{}
	p.rotor = r
	p.SetSoundEffectsEnabled(false)
	for _, id := range []uint16{0x0a, 0x2a} {
		if err = p.PlayGS2Event(id, 0); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(r.fixed, []uint16{0x0a, 0x2a}) {
		t.Fatal("mute lost source transport", r.fixed)
	}
	p.SetEffectsPaused(true)
	p.PlayGS2Event(0x0a, 0)
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(true)
	if len(r.fixed) != 2 {
		t.Fatal("pause advanced defense command")
	}
	if err = p.PlayGS2Event(0x2a, 0); err != nil || len(r.fixed) != 3 {
		t.Fatal("resume lost active retained transport", err)
	}
}
