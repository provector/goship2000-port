package cockpit

import "fmt"

// MapObjective is the display subset of one source62-byte TARGET record.
// Status is its +32 display/status word, not the raw objective-kind enum.
type MapObjective struct {
	Status                 int16
	H0, H1                 uint32
	EndpointH0, EndpointH1 uint32
	EndpointDirection      uint16
}

// MapMission separates known mission anchors from the evolving contact list.
// FARPCount is TARGET+50; FARPLosses is the distinct signed DATA:AF1 owner.
type MapMission struct {
	Base, FARP, Primary, Secondary MapObjective
	PassThrough                    bool
	FARPCount, FARPLosses          int16
}

// DrawMissionMarkers is resident015b:02f8..06d3, shared by GPS and cockpit maps.
// Callers supply their own source projection, divisor and vector clip semantics.
// It consumes an immutable snapshot and never reveals unsighted world entities.
func (a *Assets) DrawMissionMarkers(dst []byte, in *MapMission, project func(uint32, uint32) (int, int), verticalDivisor int32, shift uint, clip Rect) error {
	if in == nil {
		return nil
	}
	if verticalDivisor == 0 {
		return fmt.Errorf("map mission requires a nonzero vertical divisor")
	}
	draw := func(name string, o MapObjective, angle uint16) error {
		vectors, ok := a.MapVectors[name]
		if !ok {
			return fmt.Errorf("prepared mission vector%s missing", name)
		}
		x, y := project(o.H0, o.H1)
		// Source adds0200 to the vector style: a(+1,+1) black pass,
		// followed by original literal colours. It does not enlarge the glyph.
		for _, v := range vectors {
			v.Colour = 0
			a.DrawVectors(dst, []Vector{v}, x+1, y+1, angle, shift, 0, clip)
		}
		a.DrawVectors(dst, vectors, x, y, angle, shift, 0, clip)
		return nil
	}
	if o := in.Base; o.Status != 0 {
		if in.PassThrough {
			x, y := project(o.H0, o.H1)
			// The source deliberately uses the vertical divisor for BOTH arms.
			// Keep this screen-space corridor mark distinct from world boundaries.
			a, b := int(int32(-65536)/verticalDivisor), int(int32(65536)/verticalDivisor)
			if int32(o.H0) >= 0x8000 && int32(o.H0) <= 0x78000 {
				Line(dst, x+a, y, x+b, y, 1, clip)
			} else {
				Line(dst, x, y+a, x, y+b, 1, clip)
			}
		}
		if err := draw("map-07df", o, 0); err != nil {
			return err
		}
	}
	if in.FARP.Status != 0 && in.FARPCount > in.FARPLosses {
		if err := draw("map-07c4", in.FARP, 0); err != nil {
			return err
		}
	}
	for _, row := range []struct {
		o    MapObjective
		name string
	}{{in.Primary, "map-075a"}, {in.Secondary, "map-0722"}} {
		if row.o.Status == 0 {
			continue
		}
		if row.o.EndpointH0 != 0 {
			if err := draw("map-081e", row.o, row.o.EndpointDirection); err != nil {
				return err
			}
		}
		if err := draw(row.name, row.o, 0); err != nil {
			return err
		}
	}
	return nil
}
