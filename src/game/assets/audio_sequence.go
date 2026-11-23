package assets

const AudioBakePolicy = "adlib-baked-canonical-v1-sourceRNG04d2"

type AudioSourceRange struct {
	Start int `json:"start"`
	End   int `json:"end_exclusive"`
}
