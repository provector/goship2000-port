package assets

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/cockpit"
)

// Source GS2 SHA1a1db960... selected profile; section5 file2f7d0, rowtable46a3.
// These are the embedded CS-relative strings used by458d, not a guessed merge
// with editor OBJDBASE descriptions. Keeping them in memory leaves all prepared
// files/fingerprints unchanged and requires no Preparation32 regeneration.
func loadFlightFeedbackTables(raw []byte, a *cockpit.Assets) error {
	if len(raw) != 413971 || a == nil {
		return fmt.Errorf("flight feedback source extent")
	}
	text := func(at, limit int) (string, error) {
		if at < 0 || at >= len(raw) {
			return "", fmt.Errorf("flight feedback string offset%x", at)
		}
		end := at
		for end < len(raw) && end-at < limit && raw[end] != 0 {
			if raw[end] < 32 || raw[end] > 126 {
				return "", fmt.Errorf("flight feedback text byte%x at%x", raw[end], end)
			}
			end++
		}
		if end == len(raw) || end-at >= limit {
			return "", fmt.Errorf("flight feedback unterminated text at%x", at)
		}
		return string(raw[at:end]), nil
	}
	a.TargetDossiers = make(map[int][4]string, 230)
	for model := 0; model < 230; model++ {
		var row [4]string
		for word := range row {
			at := int(binary.LittleEndian.Uint16(raw[0x2f7d0+0x46a3+model*8+word*2:]))
			if at >= 0x46a3 {
				return fmt.Errorf("target dossier model%d word%d: CS text offset%x outside text region", model, word, at)
			}
			var err error
			row[word], err = text(0x2f7d0+at, min(512, 0x46a3-at))
			if err != nil {
				return fmt.Errorf("target dossier model%d word%d: %w", model, word, err)
			}
		}
		a.TargetDossiers[model] = row
	}
	a.MemberCaptionLiterals = map[string]string{}
	for name, ip := range map[string]int{"formed": 0x16e, "holding": 0x17c, "fly": 0x188, "regroup": 0x193, "disengage": 0x1a3, "landing": 0x1b4, "base": 0x1c0, "landed": 0x1cc, "out": 0x235, "altitude": 0x226, "speed": 0x200} {
		s, e := text(0x487f0+(0x2a29-0x272d)*16+ip, 80)
		if e != nil {
			return e
		}
		a.MemberCaptionLiterals[name] = s
	}
	return nil
}
