package State

import "testing"

func TestCloneMovementStateIsolatesProtocolLeaderIdentities(t *testing.T) {
	id, dlid, wid := int64(-1), int64(-14), int64(2)
	original := MovementState{ID: 51, LeaderID: &id, LeaderDLID: &dlid, LeaderWID: &wid}
	clone := cloneMovementState(original)
	*clone.LeaderID = 7
	*clone.LeaderDLID = 8
	*clone.LeaderWID = 9
	if *original.LeaderID != -1 || *original.LeaderDLID != -14 || *original.LeaderWID != 2 {
		t.Fatal("cloned protocol leader mutation changed original movement")
	}
	if clone.CommanderID != nil {
		t.Fatal("protocol leader created an owned commander")
	}
	empty := cloneMovementState(MovementState{})
	if empty.LeaderID != nil || empty.LeaderDLID != nil || empty.LeaderWID != nil {
		t.Fatal("missing protocol leader identity was invented")
	}
}
