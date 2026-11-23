package assets

// The prepared metadata distinguishes source identity, canonical render policy,
// source busy state and PCM lifetime. None of it is a saved gameplay preference.
type AudioMetadata struct {
	Schema         int                    `json:"audio_schema_version"`
	Profile        string                 `json:"source_profile"`
	Policy         string                 `json:"policy"`
	Component      any                    `json:"component"`
	Tracks         []AudioTrackMetadata   `json:"tracks"`
	Carriers       []AudioCarrierMetadata `json:"carriers"`
	ContextSources []SourceLocation       `json:"interpretation_sources"`
	DigitalEvents  []AudioDigitalEvent    `json:"digital_events"`
	GS2Commands    []AudioGS2Command      `json:"gs2_fixed_gameplay_commands"`
	Files          []Output               `json:"files"`
	Adaptations    []string               `json:"adaptations"`
}

// AudioGS2Command is compiled during preparation from the recognized module's
// constructor. Playback needs typed row/stream data, never MZ instructions.
type AudioGS2Command struct {
	EventID                   uint16 `json:"source_event_id"`
	Channel, Start, Alternate int
	HandlerCS, HandlerFile    int
	HandlerSHA256, HandlerHex string
	TrackKey                  string `json:"isolated_fallback_track"`
	ClearNoise                bool   `json:"clear_retained_noise,omitempty"`
}

type AudioCarrierMetadata struct {
	Name            string         `json:"name"`
	Source          SourceLocation `json:"source"`
	Raw             Output         `json:"raw"`
	InitializedData *Output        `json:"initialized_data,omitempty"`
	Tables          *Output        `json:"tables,omitempty"`
}

type AudioTrackMetadata struct {
	Key                     string             `json:"key"`
	Module                  string             `json:"module"`
	EventID                 int                `json:"source_event_id"`
	Aliases                 []int              `json:"aliases,omitempty"`
	Output                  Output             `json:"output"`
	SampleRate              int                `json:"sample_rate"`
	Bits                    int                `json:"bits"`
	Channels                int                `json:"channels"`
	Frames                  uint64             `json:"frames"`
	BusyEndFrame            uint64             `json:"logical_busy_end_frame"`
	LastNonzeroFrame        int64              `json:"last_nonzero_frame"`
	TailEndFrame            uint64             `json:"tail_end_frame"`
	ClippedSamples          uint64             `json:"clipped_samples"`
	OutputGain              int                `json:"output_gain"`
	SilenceValue            int                `json:"silence_value"`
	PCMSHA256               string             `json:"pcm_sha256"`
	LoopStartFrame          uint64             `json:"loop_start_frame,omitempty"`
	LoopEndFrame            uint64             `json:"loop_end_exclusive_frame,omitempty"`
	SourcePeriodPIT         uint64             `json:"source_period_pit_clocks,omitempty"`
	SourcePeriodDenominator uint64             `json:"source_period_denominator,omitempty"`
	SourceStartPIT          uint64             `json:"source_start_pit_clocks,omitempty"`
	BusyEndPIT              uint64             `json:"logical_busy_end_pit_clocks"`
	RNGSeed                 uint16             `json:"rng_seed"`
	ClockPolicy             string             `json:"clock_policy"`
	PreparationPolicy       string             `json:"preparation_policy,omitempty"`
	SourceRanges            []AudioSourceRange `json:"source_stream_ds_ranges,omitempty"`
	Sequence                Output             `json:"source_sequence_metadata"`
}

type AudioDigitalBlock struct {
	EndWord, StartWord, ParagraphWord uint16
	BankStart                         int    `json:"bank_start"`
	BankEnd                           int    `json:"bank_end_exclusive"`
	DescriptorFileOffset              int    `json:"descriptor_file_offset"`
	PCMHash                           string `json:"pcm_sha256"`
}

type AudioDigitalEvent struct {
	EventID           int                 `json:"source_event_id"`
	HandlerCS         int                 `json:"handler_cs"`
	Priority          int                 `json:"priority"`
	CompanionPriority int                 `json:"companion_priority"`
	Blocks            []AudioDigitalBlock `json:"blocks"`
	TrackKey          string              `json:"track_key"`
	AssemblyPolicy    string              `json:"assembly_policy"`
}
