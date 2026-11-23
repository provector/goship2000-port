package assets

// Coordinates belong only to the recognized GS source profile. Overlay IPs
// are not DOS runtime segments; production preparation reads source bytes.
type DebriefSourceLayout struct {
	RankThresholdsDS, MedalThresholdsDS, MedalPenaltyCreditsDS    int
	RankNamesDS, MedalCitationsDS, WingQualifiersDS, WingLevelsDS int
	TextFirstDS, TextEndDS                                        int
	Section3File, FeedbackBankFile, FeedbackBankSegment           int
}

var islandIceDebriefLayout = DebriefSourceLayout{
	RankThresholdsDS: 0x5204, MedalThresholdsDS: 0x58be, MedalPenaltyCreditsDS: 0x58ce,
	RankNamesDS: 0x0c0e, MedalCitationsDS: 0x58ae, WingQualifiersDS: 0x5582, WingLevelsDS: 0x5588,
	TextFirstDS: 0x4d44, TextEndDS: 0x58ae, Section3File: 0x369c0, FeedbackBankFile: 0x3b360, FeedbackBankSegment: 0x293f,
}
