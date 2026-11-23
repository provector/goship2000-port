package assets

import (
	"testing"
)

func TestOverallProgressMonotonicAndFinal(t *testing.T) {
	var work PreparationWork
	if p := work.Observe(PreparationProgress{Stage: "Inventory original installation"}); p.OverallTotal != 0 {
		t.Fatal(p)
	}
	last := 0.0
	for i, stage := range preparationStages {
		for n := 0; n <= 8; n++ {
			p := work.Observe(PreparationProgress{Stage: stage, Completed: n, Total: 8})
			f := p.Fraction()
			if f < last || f > 1 || f == 1 && (i != len(preparationStages)-1 || n != 8) {
				t.Fatal(p, last)
			}
			last = f
		}
	}
	if last != 1 {
		t.Fatal(last)
	}
}
