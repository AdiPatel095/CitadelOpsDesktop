package GameParser

import (
	"testing"
	"time"

	"CitadelDesktop/Server/Models"
)

func resetGAMSnapshotTestState() {
	gamSnapshotAssembly.Lock()
	if gamSnapshotAssembly.timer != nil {
		gamSnapshotAssembly.timer.Stop()
	}
	gamSnapshotAssembly.pending = nil
	gamSnapshotAssembly.pendingObservedUnix = 0
	gamSnapshotAssembly.assembling = false
	gamSnapshotAssembly.generation++
	gamSnapshotAssembly.timer = nil
	gamSnapshotAssembly.deferred = nil
	gamSnapshotAssembly.deferredObservedUnix = 0
	gamSnapshotAssembly.deferredValid = false
	gamSnapshotAssembly.Unlock()
	completeGAMRequest()
}

func TestGAMSnapshotPagesReplaceAndFilterByOwner(t *testing.T) {
	resetGAMSnapshotTestState()
	defer resetGAMSnapshotTestState()

	gs := Models.GetGameState()
	gs.Reset()
	gs.PlayerID = 77
	now := time.Now().Unix()
	gs.Movement.ReplaceSnapshot([]Models.GAMMovement{{
		MID:          999,
		OID:          77,
		CommanderID:  99,
		ReceivedUnix: now,
	}}, now)

	pageOne := make([]Models.GAMMovement, 0, gamSnapshotPageSize)
	for i := 0; i < gamSnapshotPageSize; i++ {
		ownerID := 77
		if i == 12 {
			ownerID = 88
		}
		pageOne = append(pageOne, Models.GAMMovement{
			MID:          i + 1,
			OID:          ownerID,
			CommanderID:  i,
			ReceivedUnix: now,
		})
	}
	queueGAMSnapshotPage([]Models.GAMMovement{{
		MID:          200,
		OID:          77,
		CommanderID:  40,
		ReceivedUnix: now,
	}}, 1, now)

	partial, _, _, _ := gs.Movement.Snapshot()
	if len(partial) != 1 || partial[0].MID != 999 {
		t.Fatalf("partial page changed live state: %+v", partial)
	}

	queueGAMSnapshotPage(pageOne, gamSnapshotPageSize, now)
	queueGAMSnapshotPage(pageOne, gamSnapshotPageSize, now)
	queueGAMSnapshotPage([]Models.GAMMovement{{
		MID:          200,
		OID:          77,
		CommanderID:  40,
		ReceivedUnix: now,
	}}, 1, now)
	queueGAMSnapshotPage(nil, 0, now)
	gamSnapshotAssembly.Lock()
	generation := gamSnapshotAssembly.generation
	gamSnapshotAssembly.Unlock()
	commitQueuedGAMSnapshot(generation)

	complete, _, ready, snapshotUnix := gs.Movement.Snapshot()
	if !ready || snapshotUnix != now {
		t.Fatalf("snapshot metadata ready=%v unix=%d, want ready at %d", ready, snapshotUnix, now)
	}
	if len(complete) != 25 {
		t.Fatalf("owned movement count = %d, want 25", len(complete))
	}
	for _, movement := range complete {
		if movement.OID != 77 {
			t.Fatalf("foreign movement admitted: %+v", movement)
		}
		if movement.MID == 999 {
			t.Fatal("movement absent from authoritative snapshot was retained")
		}
	}

	queueGAMSnapshotPage(nil, 0, now+1)
	gamSnapshotAssembly.Lock()
	emptyGeneration := gamSnapshotAssembly.generation
	gamSnapshotAssembly.Unlock()
	commitQueuedGAMSnapshot(emptyGeneration)
	empty, _, _, _ := gs.Movement.Snapshot()
	if len(empty) != 0 {
		t.Fatalf("standalone empty snapshot retained %d movements", len(empty))
	}
}
