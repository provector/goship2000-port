package menus

// These are GS4 driver command numbers, not cross-module sound names. Startup
// LOG/GS3 ownership is handled separately by playback; render/hover never emit.
const (
	gs4Computer = uint16(0x0d)
	gs4Switch   = uint16(0x0e)
	gs4Knob     = uint16(0x13) // Source alias of0dh; not19h.
	gs4Denied   = uint16(0x21)
	gs4Accepted = uint16(0x22)
	gs4Entry    = uint16(0x23)
	gs4Page     = uint16(0x24)
	gs4Door     = uint16(0x42)
)

func (c *FrontEnd) emitAudioCue(id uint16) { c.audioCues = append(c.audioCues, id) }

func (c *FrontEnd) takeAudioCues() []uint16 {
	cues := c.audioCues
	c.audioCues = nil
	return cues
}
