package sounds

import assetdata "gunship2000-assets/game/assets"

type gs2DigitalVoice struct {
	event uint16
	asset assetdata.AudioDigitalPlayback
}

// ASOUND0db1 keeps two priority-sorted pending lists, separate from the active
// sample list. Higher priority does not interrupt an already playing utterance.
func (p *AudioPlayer) queueGS2Digital(event uint16, asset assetdata.AudioDigitalPlayback) {
	incoming := &gs2DigitalVoice{event: event, asset: asset}
	if p.digitalPending[0] == nil || asset.Priority > p.digitalPending[0].asset.Priority {
		p.digitalPending[1] = p.digitalPending[0]
		p.digitalPending[0] = incoming
	} else if p.digitalPending[1] == nil || asset.Priority > p.digitalPending[1].asset.Priority {
		p.digitalPending[1] = incoming
	}
	if p.digitalCurrent == nil {
		p.startGS2Digital()
	}
}

func (p *AudioPlayer) startGS2Digital() {
	p.digitalCurrent = p.digitalPending[0]
	p.digitalPending[0], p.digitalPending[1] = p.digitalPending[1], nil
	p.digitalFrames = 0
}

func (p *AudioPlayer) clearGS2Digital() {
	p.digitalCurrent = nil
	p.digitalPending = [2]*gs2DigitalVoice{}
	p.digitalFrames = 0
}

func (p *AudioPlayer) gs2DigitalSample() float64 {
	if p.effectsPaused {
		return 0
	}
	for p.digitalCurrent != nil {
		clip := p.digitalCurrent.asset.Clip
		// Exact rational zero-order hold preserves the selected8000Hz unsigned
		// payload at the49716Hz device clock; no pitch shift or hidden filter.
		at := p.digitalFrames * uint64(clip.SampleRate) / assetdata.AudioSampleRate
		if at >= uint64(len(clip.Samples)) {
			p.startGS2Digital()
			continue
		}
		p.digitalFrames++
		if !p.effectsEnabled || p.prefs.SpeechMuted {
			return 0
		}
		return float64(clip.Samples[at])
	}
	return 0
}
