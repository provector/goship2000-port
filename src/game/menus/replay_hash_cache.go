package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	gamestate "gunship2000-assets/game/state"
)

// This private playback cache stores value snapshots, never expected film hashes.
// InitializedEntity is comparable and contains no mutable references.
type entityJSON struct {
	value   gamestate.InitializedEntity
	encoded []byte
}
type replayHashCache struct {
	entities        []entityJSON
	reference       []byte
	bottom          int
	referenceDigest string
}

func (c *replayHashCache) missionHash(m *gamestate.InitializedMission) string {
	if c == nil || m == nil || len(m.Entities) == 0 || len(m.Entities) > 1024 {
		return filmHash(m)
	}
	// Marshal all other fields with the canonical encoder. Reinsert the canonical
	// array exactly where that encoder placed it, falling back on any ambiguity.
	rest := *m
	rest.Entities = nil
	b := filmJSON(&rest)
	key := []byte(`"Entities":null`)
	pos := bytes.Index(b, key)
	if pos < 0 || bytes.Index(b[pos+len(key):], key) >= 0 {
		return filmHash(m)
	}
	if len(c.entities) != len(m.Entities) {
		c.entities = make([]entityJSON, len(m.Entities))
	}
	h := sha256.New()
	h.Write(b[:pos+len(key)-4])
	h.Write([]byte("["))
	for i, v := range m.Entities {
		entry := &c.entities[i]
		if entry.encoded == nil || entry.value != v {
			entry.encoded = filmJSON(v)
			entry.value = v
		}
		if i != 0 {
			h.Write([]byte(","))
		}
		h.Write(entry.encoded)
	}
	h.Write([]byte("]"))
	h.Write(b[pos+len(key):])
	return hex.EncodeToString(h.Sum(nil))
}
func (c *replayHashCache) referenceHash(buf []byte, bottom int) string {
	if c != nil && c.referenceDigest != "" && c.bottom == bottom && (c.reference == nil) == (buf == nil) && bytes.Equal(buf, c.reference) {
		return c.referenceDigest
	}
	result := filmHash(struct {
		Buffer []byte
		Bottom int
	}{buf, bottom})
	if c != nil && len(buf) <= 64000 {
		c.reference = bytes.Clone(buf)
		c.bottom = bottom
		c.referenceDigest = result
	}
	return result
}
