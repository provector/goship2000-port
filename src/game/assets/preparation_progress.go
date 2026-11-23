package assets

// Local counters describe actual resource/job visits. Overall stage work uses
// one completed stage-unit per pipeline stage, plus its real local fraction.
// It measures work, not elapsed time. Discovery stays indeterminate.
type PreparationProgress struct {
	Stage            string  `json:"stage"`
	Completed        int     `json:"completed"`
	Total            int     `json:"total"`
	OverallCompleted float64 `json:"overall_completed"`
	OverallTotal     int     `json:"overall_total"`
}

var preparationStages = []string{"Extract source resources", "Decode pictures and palettes", "Office, fonts and menus", "Startup animation", "Mission and model tables", "Debrief scenes", "World geometry and maps", "Encode world geometry", "Replay metadata", "Editor metadata", "Sound conversion", "Write manifest", "Validate current prepared capabilities", "Publish prepared cache", "Prepared cache ready"}

type PreparationWork struct {
	known bool
	last  float64
}

func (w *PreparationWork) Observe(p PreparationProgress) PreparationProgress {
	index := -1
	for i, s := range preparationStages {
		if s == p.Stage {
			index = i
			break
		}
	}
	if index < 0 || !w.known && index != 0 {
		return p
	}
	if index == 0 && p.Total > 0 {
		w.known = true
	}
	if !w.known {
		return p
	}
	n := float64(index)
	if p.Total > 0 {
		n += float64(max(0, min(p.Completed, p.Total))) / float64(p.Total)
	}
	// Substage completion can coalesce; stages never regress or restart the bar.
	w.last = max(w.last, n)
	p.OverallCompleted = w.last
	p.OverallTotal = len(preparationStages)
	return p
}
func (p PreparationProgress) Fraction() float64 {
	if p.OverallTotal == 0 {
		return 0
	}
	return min(1, max(0, p.OverallCompleted/float64(p.OverallTotal)))
}
