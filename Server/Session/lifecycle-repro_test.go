package Session

import "testing"

func TestStopStartCanDuplicateHeadAndRemoveFollowingFrameRegression(t *testing.T) {
	testOutboxImmediateRestart(t)
}
