package configuration

import (
	"fmt"
	"strconv"
)

// RecordingLimit is operational storage policy. It is deliberately separate
// from RuntimeOptions, which historical films freeze as simulation input.
type RecordingLimit int64

const DefaultReplayMaxMiB = 128

// Maximum stored bytes of a complete compressed container, including its index and ending.
const MaximumReplayMaxMiB = 1024
const ReplayReadMaxBytes = MaximumReplayMaxMiB * 1024 * 1024

func ParseRecordingLimit(text string) (RecordingLimit, error) {
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 1 || n > MaximumReplayMaxMiB {
		return 0, fmt.Errorf("replay-max-mib must be a positive integer in1..%d", MaximumReplayMaxMiB)
	}
	return RecordingLimit(n * 1024 * 1024), nil
}

func (l RecordingLimit) Effective() RecordingLimit {
	if l == 0 {
		return DefaultReplayMaxMiB * 1024 * 1024
	}
	return l
}
func (l RecordingLimit) Validate() error {
	if l < 1 || l > ReplayReadMaxBytes || l%(1024*1024) != 0 {
		return fmt.Errorf("recording limit must be whole MiB in1..%d", MaximumReplayMaxMiB)
	}
	return nil
}
