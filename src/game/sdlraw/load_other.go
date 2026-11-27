//go:build !linux && !windows

package sdlraw

func bindGUID(uintptr) func(int32) guid { return nil }
