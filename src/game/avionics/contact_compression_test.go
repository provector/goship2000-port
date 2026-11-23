package avionics

import "testing"

// Full contact Advance, including source scan/shared draws/body LOS and the
// actual independent follower tail. This supplies geometry, not the outcome.
func TestIndependentObjectiveContactReachesCompressionCaller(t *testing.T) {
	for _, flags := range []byte{0, 0x10, 0x20, 0x30, 0x40} {
		s, in, target := tailFixture(t)
		for actor := 2; actor < 5; actor++ {
			in.Entities[actor].Control[1] = 0
		}
		in.Entities[1].Control[1] = 2
		in.Entities[10].Kind = 0x15
		in.Entities[10].Object[0] = flags
		// Cargo is outside the player's envelope but inside the follower's.
		in.Entities[10].Pose.H1 = in.Player.H1 + 32768
		in.Entities[1].Pose = in.Entities[10].Pose
		in.Entities[1].Pose.H1 -= 2048
		s.Contacts[10] = Contact{}
		calls := 0
		in.ContactOrder = func(actor, row int) error {
			if actor != 1 || row != 10 {
				t.Fatalf("wrong contact owner %d/%d", actor, row)
			}
			calls++
			return nil
		}
		if err := s.Advance(in); err != nil {
			t.Fatal(err)
		}
		if calls != 1 || s.Contacts[10].Flags&7 != 6 || target[1] != 0 {
			t.Fatalf("not an independently reached cargo contact: calls%d target%d flags%02x", calls, target[1], s.Contacts[10].Flags)
		}
		if s.CompressionLimited != (flags&0x30 != 0) {
			t.Fatalf("flags%02x limit%v", flags, s.CompressionLimited)
		}
	}
}
