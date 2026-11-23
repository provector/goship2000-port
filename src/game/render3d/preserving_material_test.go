package render3d

import (
	"reflect"
	"testing"
)

func TestZeroLowMaterialSourceLeaf(t *testing.T) {
	// Independently hand-executed VGA04b3/04bb/04c4: even row+even start
	// writes high, odd start first skips one; SHR counts floor(n/2).
	for _, q := range []struct {
		y, l, r int
		want    []byte
	}{
		{0, 0, 3, []byte{236, 14, 236, 14}},
		{0, 0, 2, []byte{236, 14, 14, 14}},
		{0, 1, 3, []byte{14, 14, 236, 14}},
		{1, 0, 3, []byte{14, 236, 14, 236}},
		{1, 0, 0, []byte{14, 14, 14, 14}},
	} {
		dst := make([]byte, 64000)
		for x := 0; x < 4; x++ {
			dst[q.y*320+x] = 14
		}
		fillSpan(dst, q.y, q.l, q.r, 0xec00, 0, nil)
		if got := dst[q.y*320 : q.y*320+4]; !reflect.DeepEqual(got, q.want) {
			t.Fatalf("%+v got%v", q, got)
		}
	}
}
