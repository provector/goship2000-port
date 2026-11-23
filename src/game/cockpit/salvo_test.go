package cockpit

import "testing"

func TestRocketSalvoOnlyChangesPrimaryRocketCaption(t *testing.T) {
	s := Station{Name: "HYDRA", DefinitionFlags: 0x1000, Quantity: 38}
	for _, size := range []int16{1, 2, 4} {
		want := "HYDRA x" + string(byte('0'+size))
		if got := primaryWeaponName(s, size); got != want {
			t.Fatal(got, want)
		}
	}
	if s.Name != "HYDRA" || s.Quantity != 38 {
		t.Fatal("render formatted by mutating authoritative name/stores")
	}
	s.DefinitionFlags = 1
	if got := primaryWeaponName(s, 4); got != "HYDRA" {
		t.Fatal("nonrocket caption appended", got)
	}
	s.DefinitionFlags = 0x1000
	if got := primaryWeaponName(s, 0); got != "HYDRA" {
		t.Fatal("uninitialized supplied state invented a salvo", got)
	}
}
