//go:build !((linux || darwin) && (amd64 || arm64)) && !(windows && amd64)

package audioopl

import (
	"fmt"
	"runtime"
)

func nativePayload() ([]byte, error) {
	return nil, fmt.Errorf("audio preparation is not packaged for %s/%s; use a supported package to prepare assets, then copy the complete prepared tree", runtime.GOOS, runtime.GOARCH)
}
