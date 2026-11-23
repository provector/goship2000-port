package avionics

import "testing"

func TestMovingReportsReplaceOnlyTheirOwnRow(t *testing.T) {
	for _, kind := range []byte{0, 5} { // helicopter and ground unit
		s := New()
		for _, row := range []int{1, 2} {
			e := Entity{Present: true, Row: row, Kind: kind, Pose: Pose{H0: 100, H1: 200}}
			observe(&s.Contacts[row], e, 0)
		}
		blocked(&s.Contacts[1])
		observe(&s.Contacts[1], Entity{Row: 1, Kind: kind, Pose: Pose{H0: 400, H1: 500}}, 3)
		blocked(&s.Contacts[1])
		count := 0
		for _, c := range s.Contacts {
			if c.Flags&7 != 0 {
				count++
			}
		}
		if count != 2 || s.Contacts[1].Flags&7 != 1 || s.Contacts[1].H0 != 400 || s.Contacts[1].H1 != 500 || s.Contacts[2].Flags&7 != 2 || s.Contacts[2].H0 != 100 || s.Contacts[2].H1 != 200 {
			t.Fatal(kind, s.Contacts[:3])
		}
	}
}
