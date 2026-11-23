package flight

import (
	"bytes"
	"encoding/hex"
	"errors"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestSourcePredecessorWriterOperands(t *testing.T) {
	_, _ = sourceFixture(t) // Uses the established optional-original fixture gate.
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, literal := range map[int]string{
		0x1542: "0ee89c10",             // Actual25E2 far-sized return frame.
		0x157f: "9abc3bbb10",           // Immediately following B01C far call.
		0x326b: "f7f98946d00bc07e05",   // IDIV then BP-30 store, positive-only branch.
		0x3274: "d1f88946d0",           // Positive quotient SAR1 then same-word store.
		0x335b: "c1f8058946d03b06e400", // Easy SAR5/store before eased E4 comparison.
	} {
		want, err := hex.DecodeString(literal)
		if err != nil || !bytes.Equal(raw[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source section1 0975:%04x changed: %v", ip, err)
		}
	}
}

func TestPhysicsTraceIsActualPreEasingWriter(t *testing.T) {
	p, q := sourceFixture(t)
	// Source APA table[31]=1765, STAT30=1486 and STAT46=22. The first
	// E0 visit eases thrust0->882 before the actual BP-30 writers:
	// easy signed(-604) SAR5=-19; realistic882/44=20 SAR1=10;
	// loaded realistic(882-2000)/44=-25, with NO positive-only SAR1.
	for _, tc := range []struct {
		name              string
		switches          uint16
		mass, want, eased int16
	}{
		{"easy", 0, 0, -19, -2},
		{"realistic-positive", 4, 0, 10, 2},
		{"realistic-negative", 4, 16000, -25, -2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Rotor: 1, Collective: 31}
			s, old := initial, initial
			in := Input{DF: 10, E0: true, Phase: 1, Switches: tc.switches, MassTerm: tc.mass}
			trace, err := StepWithTrace(&s, p, in, q)
			if err != nil {
				t.Fatal(err)
			}
			if !trace.DesiredVerticalRateWritten || trace.DesiredVerticalRate != tc.want || s.Vertical != tc.eased || trace.DesiredVerticalRate == s.Vertical {
				t.Fatal("pre-easing source writer lost", trace, s.Vertical)
			}
			if err = Step(&old, p, in, q); err != nil || !reflect.DeepEqual(s, old) {
				t.Fatal("trace changed authoritative physics", err, s, old)
			}
		})
	}
}

func TestPhysicsTraceSkippedOrFailedVisitHasNoWriter(t *testing.T) {
	p, q := sourceFixture(t)
	base := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Rotor: 1, Collective: 31, Vertical: 7}
	for _, tc := range []struct {
		name     string
		terminal *Terminal
		e0       bool
	}{
		{"E0false", nil, false},
		{"terminal", &Terminal{Kind: "retained-test-terminal"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			s.Terminal = tc.terminal
			trace, err := StepWithTrace(&s, p, Input{DF: 6, Phase: 1, E0: tc.e0}, q)
			if err != nil || trace != (StepTrace{}) {
				t.Fatal("skipped source writer invented", trace, err)
			}
		})
	}
	// The writer is reached before contact, but a failed transactional visit
	// may neither partially move the player nor donate its uncommitted trace.
	s := base
	q.Height = func(Pose) (int16, error) { return 0, errors.New("rejected contact fixture") }
	trace, err := StepWithTrace(&s, p, Input{DF: 6, E0: true, Phase: 1}, q)
	if err == nil || trace != (StepTrace{}) || !reflect.DeepEqual(s, base) {
		t.Fatal("failed visit published a stale/partial writer", trace, err, s)
	}
}

func TestEasyWriterPreservesSignedWordSubtraction(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct{ thrust, want int16 }{{-32768, 977}, {32767, 977}, {-31282, -1024}, {1486, 0}} {
		s := State{Thrust: tc.thrust}
		k := kernel{p: p, q: q}
		k.easyMotion(&s, 0, 0)
		if k.err != nil || !k.trace.DesiredVerticalRateWritten || k.trace.DesiredVerticalRate != tc.want {
			t.Fatal("335E wrapped SUB/SAR mismatch", tc, k.trace, k.err)
		}
	}
}
