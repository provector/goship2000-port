package menus

import "testing"

func TestOutfitEscapeReturnsOnlyFromSummary(t *testing.T) {
	for _, mode := range []string{"pilot", "helicopter", ""} {
		v := OutfitView{Mode: mode}
		result, err := v.Handle(Actions{Cancel: true})
		if err != nil || ((result.Kind == "return") != (mode == "")) || v.Mode != "" {
			t.Fatal(mode, result, err)
		}
	}
}
