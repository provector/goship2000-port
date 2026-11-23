package sounds

import (
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func TestRotorSourceHistoryAndStartupHandoff(t *testing.T) {
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, e := rotorSequence(source[l.DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	if e = q.rotorCommand(1); e != nil {
		t.Fatal(e)
	}
	ref, e := newAudioSequence(source, l, assetdata.IslandIce469085.AudioLayout)
	if e != nil {
		t.Fatal(e)
	}
	if e = ref.startInitializationChannel(); e != nil {
		t.Fatal(e)
	}
	// The accepted initial ramp must be exactly the same source sequence before
	// the old bounded bake deliberately inserts reset/release at the first21.
	for n := 0; n < 30000; n++ {
		q.interrupt()
		ref.interrupt()
		if q.err != nil || ref.err != nil {
			t.Fatal(q.err, ref.err)
		}
		if !reflect.DeepEqual(q.writes, ref.writes) || q.mem != ref.mem {
			t.Fatal("late initialization changed before source steady state")
		}
		q.writes = q.writes[:0]
		ref.writes = ref.writes[:0]
		if q.word(l.NoiseSpeed) == 21 && q.word(l.NoiseStep) == 0 {
			break
		}
		if n == 29999 {
			t.Fatal("source ramp bound")
		}
	}
	row := q.mem[q.base(6) : q.base(6)+30]
	before := append([]byte(nil), row...)
	if e = q.rotorCommand(15); e != nil {
		t.Fatal(e)
	}
	if q.word(l.NoiseSpeed) != 21 || q.word(l.NoiseStep) != 0 || q.mem[l.NoiseLevel] != 57 || !reflect.DeepEqual(before, row) {
		t.Fatal("positive update reset retained speed/row or wrong SHR1 level")
	}
	fresh, e := rotorSequence(source[l.DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	if e = fresh.rotorCommand(15); e != nil {
		t.Fatal(e)
	}
	if fresh.word(l.NoiseSpeed) != 0 || fresh.word(l.NoiseStep) != 1 {
		t.Fatal("fresh and retained histories accidentally equivalent")
	}
	if e = q.rotorCommand(0); e != nil {
		t.Fatal(e)
	}
	if q.word(l.NoiseStep) != 0xffff || q.mem[l.NoiseLevel] != 50 {
		t.Fatal("source shutdown step/level")
	}
	for i := 0; i < 30000 && q.word(l.NoiseKind) != 0; i++ {
		q.interrupt()
	}
	if q.err != nil || q.word(l.NoiseKind) != 0 || q.word(l.NoiseStep) != 0 || q.word(l.NoiseSpeed) != 0 {
		t.Fatal("source ramp-down did not terminate", q.err)
	}
	if e = q.rotorCommand(16); e == nil {
		t.Fatal("unverified argument accepted")
	}
}
