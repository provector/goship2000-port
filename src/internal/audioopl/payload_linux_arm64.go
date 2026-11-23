package audioopl

import _ "embed"

//go:embed bin/audio-bake-linux-arm64.gz
var helperCompressed []byte

func nativePayload() ([]byte, error) { return helperCompressed, nil }
