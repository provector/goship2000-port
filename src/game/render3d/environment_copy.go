package render3d

import "slices"

func (e *EnvironmentState) Copy() *EnvironmentState {
	if e == nil {
		return nil
	}
	v := *e
	v.noise = slices.Clone(e.noise)
	return &v
}
