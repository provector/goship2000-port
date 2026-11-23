package assets

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/world"
)

func prepareCombatPresentation(gs *Resource, p *AssetVersionProfile, b *world.Bundle, write func(string, []byte) error) error {
	l := p.WorldLayout.Combat
	if l == nil {
		return fmt.Errorf("combat presentation requires recognized source layout")
	}
	c := &world.CombatPresentation{Version: 2, PhaseLengths: [4]int16{64, 32, 128, 96}}
	slice := func(at, n int) ([]byte, error) {
		if at < 0 || n < 0 || at > len(gs.Payload)-n {
			return nil, fmt.Errorf("combat source slice outside profile")
		}
		raw := gs.Payload[at : at+n]
		c.Sources = append(c.Sources, world.CombatSource{FileOffset: at, Length: n, SHA256: Hash(raw)})
		return raw, nil
	}
	head, err := slice(l.HeaderFile, l.HeaderBytes)
	if err != nil {
		return err
	}
	near, err := slice(l.NearPoolFile, l.NearPoolBytes)
	if err != nil {
		return err
	}
	far, err := slice(l.FarPoolFile, l.FarPoolBytes)
	if err != nil {
		return err
	}
	c.Headers = append([]byte(nil), head...)
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("effects/projectile-%d", i)
		g, e := world.ParseEmbeddedApplication(id, head[i*38:(i+1)*38], far, near)
		if e != nil {
			return fmt.Errorf("%s:%w", id, e)
		}
		g.SourceOffset, g.SourceLength, g.SourceHash = l.HeaderFile+i*38, 38, Hash(head[i*38:(i+1)*38])
		b.Geometries[id] = g
		c.Templates[i] = id
	}
	at := l.ExplosionHeaderOffset
	if at < 0 || at+36 > len(near) {
		return fmt.Errorf("explosion header outside near pool")
	}
	g, err := world.ParseEmbeddedApplication("effects/explosion", near[at:at+36], far, near)
	if err != nil {
		return fmt.Errorf("explosion:%w", err)
	}
	g.SourceOffset, g.SourceLength, g.SourceHash = l.NearPoolFile+at, 36, Hash(near[at:at+36])
	b.Geometries[g.ID] = g
	c.Explosion = g.ID
	// History headers carry2cf5 rather than the projectile/explosion2cff
	// vertex segment. The checked paragraph delta is160bytes, not one common
	// pool for coincident near offsets (history98 is file4e508, count1).
	historyFar, err := slice(l.FarPoolFile-0xa0, l.FarPoolBytes+0xa0)
	if err != nil {
		return err
	}
	for i := 0; i < 8; i++ {
		at := 0x2b58 + 26*i
		if at+26 > len(near) {
			return fmt.Errorf("death-history header outside near pool")
		}
		header := near[at : at+26]
		id := fmt.Sprintf("effects/death-history-%d", i)
		history, e := world.ParseDeathHistoryApplication(id, header, historyFar, near)
		if e != nil {
			return fmt.Errorf("%s:%w", id, e)
		}
		history.SourceOffset, history.SourceLength, history.SourceHash = l.NearPoolFile+at, 26, Hash(header)
		b.Geometries[id] = history
		c.DeathHistory = append(c.DeathHistory, id)
		c.Sources = append(c.Sources, world.CombatSource{FileOffset: l.NearPoolFile + at, Length: 26, SHA256: Hash(header)})
	}
	at = l.ExplosionBaseOffset
	if at < 0 || at+2 > len(far) {
		return fmt.Errorf("explosion base outside far pool")
	}
	n := int(binary.LittleEndian.Uint16(far[at:]))
	if at+2+n*6 > len(far) {
		return fmt.Errorf("explosion base vertices")
	}
	for i := 0; i < n; i++ {
		var v world.Vertex
		for j := range v {
			v[j] = int16(binary.LittleEndian.Uint16(far[at+2+i*6+j*2:]))
		}
		c.ExplosionBase = append(c.ExplosionBase, v)
	}
	lookup, err := slice(l.DriverFile+l.SpriteLookupOffset, l.SpriteLookupCount*2)
	if err != nil {
		return err
	}
	for i := 0; i < l.SpriteLookupCount; i++ {
		at := l.DriverFile + int(binary.LittleEndian.Uint16(lookup[2*i:]))
		h, e := slice(at, 4)
		if e != nil {
			return e
		}
		w, hh := int(binary.LittleEndian.Uint16(h)), int(binary.LittleEndian.Uint16(h[2:]))
		if w < 1 || hh < 1 || w > 320 || hh > 200 {
			return fmt.Errorf("impact sprite dimensions")
		}
		pixels, e := slice(at+4, w*hh)
		if e != nil {
			return e
		}
		name := fmt.Sprintf("effects/impact-%02d.indices", i)
		if e = write(name, pixels); e != nil {
			return e
		}
		png, e := grayIndexPNG(pixels, w, hh)
		if e != nil {
			return e
		}
		if e = write(fmt.Sprintf("effects/impact-%02d.png", i), png); e != nil {
			return e
		}
		c.ImpactSprites = append(c.ImpactSprites, world.EffectSprite{Width: w, Height: hh, SourceOffset: at, Path: name, SHA256: Hash(pixels)})
	}
	b.Combat = c
	return nil
}
