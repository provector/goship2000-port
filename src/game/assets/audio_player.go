package assets

const AudioSampleRate = 49716

type AudioClip struct {
	Music                        *MusicPages
	Samples                      []int16
	SampleRate                   int
	BusyEndFrame                 uint64
	LoopStartFrame, LoopEndFrame uint64 // Effects loop only when explicitly selected by their semantic owner.
}

type AudioAssets struct {
	Clips         map[string]*AudioClip
	Policy        string
	ASoundGS2Data []byte // Verified prepared initialized DS carrier; never original MZ code.
	GS2Commands   map[uint16]AudioGS2Command
	GS2Digital    map[uint16]AudioDigitalPlayback
}

type AudioDigitalPlayback struct {
	Clip     *AudioClip
	Priority int
}
