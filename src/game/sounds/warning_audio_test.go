package sounds

import (
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestWarningSourceRowTwoConstructors(t *testing.T) {
	source := initializationSource(t)
	layout := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	for _, tc := range []struct {
		event, start int
		hex          string
	}{{4, 0x8d8, "8d0ed808e915fe"}, {5, 0x8d0, "8d0ed008e90efe"}} {
		c, e := prepareGS2Command(source, layout, assetdata.IslandIce469085.AudioLayout, tc.event)
		if e != nil {
			t.Fatal(e)
		}
		if c.Channel != 2 || c.Start != tc.start || c.HandlerHex != tc.hex || c.ClearNoise {
			t.Fatalf("event%d %+v", tc.event, c)
		}
		q, e := newAudioSequence(source, layout, assetdata.IslandIce469085.AudioLayout)
		if e != nil {
			t.Fatal(e)
		}
		if e = q.command(tc.event); e != nil {
			t.Fatal(e)
		}
		// Source finite stream reaches retirement rather than a made-up sustained bell.
		for n := 0; n < 30000 && q.busy() != 0; n++ {
			q.interrupt()
			if q.err != nil {
				t.Fatal(q.err)
			}
		}
		if q.busy() != 0 {
			t.Fatal("warning stream failed bounded finite completion")
		}
	}
}
