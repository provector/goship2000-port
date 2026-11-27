//go:build !linux && !windows && !darwin

package sdlraw

func bindGUID(uintptr) func(int32) guid { return nil }
