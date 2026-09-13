package State

import "testing"

func TestCloneMovementStateIsolatesProtocolLeaderIdentities(t *testing.T) {
	id, dlid := int64(-1), int64(-14)
	original := MovementState{ID: 51, LeaderID: &id, LeaderDLID: &dlid}
	clone := cloneMovementState(original)
	*clone.LeaderID = 7
	*clone.LeaderDLID = 8
	if *original.LeaderID != -1 || *original.LeaderDLID != -14 {
		t.Fatal("cloned protocol leader mutation changed original movement")
	}
	if clone.CommanderID != nil {
		t.Fatal("protocol leader created an owned commander")
	}
	empty := cloneMovementState(MovementState{})
	if empty.LeaderID != nil || empty.LeaderDLID != nil {
		t.Fatal("missing protocol leader identity was invented")
	}
}
