package State

import (
	"fmt"
	"testing"
	"time"
)

// Synthetic fixtures use the production keyed mutators, including physical shards.
func BenchmarkMapObservationSweep(b *testing.B) {
	state := NewGameState()
	var keys [4][]string
	for i := 0; i < 47000; i++ {
		kingdom := KingdomID(i % 4)
		x, y := i/4, 1
		state.SetMapObservation(MapObservation{KingdomID: kingdom, X: x, Y: y, TypeID: MapTypeForeignLord, Level: i % 100})
		keys[kingdom] = append(keys[kingdom], fmt.Sprintf("%d:%d", x, y))
	}
	b.Run("Lookup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count := 0
			for kingdom, entries := range keys {
				for _, key := range entries {
					if _, found := state.LookupMapObservation(KingdomID(kingdom), key); found {
						count++
					}
				}
			}
			if count != 47000 {
				b.Fatalf("observations=%d", count)
			}
		}
	})
	b.Run("Range", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count := 0
			for kingdom := range keys {
				state.RangeMapObservations(KingdomID(kingdom), func(_ string, _ MapObservation) bool { count++; return true })
			}
			if count != 47000 {
				b.Fatalf("observations=%d", count)
			}
		}
	})
}

func BenchmarkMovementAccessors(b *testing.B) {
	state := NewGameState()
	for id := MovementID(1); id <= 500; id++ {
		state.SetMovement(id, MovementState{ID: id, OwnerPlayerID: 1})
	}
	b.Run("Lookup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count := 0
			for id := MovementID(1); id <= 500; id++ {
				if _, found := state.LookupMovement(id); found {
					count++
				}
			}
			if count != 500 {
				b.Fatalf("movements=%d", count)
			}
		}
	})
	b.Run("Range", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count := 0
			state.RangeMovements(func(_ MovementID, _ MovementState) bool { count++; return true })
			if count != 500 {
				b.Fatalf("movements=%d", count)
			}
		}
	})
}

func BenchmarkEventScoreLookups(b *testing.B) {
	state := NewGameState()
	for id := int64(1); id <= 100; id++ {
		state.SetScalableEventScore(id, ScalableEventScore{EventID: id, PlayerScore: id * 10})
	}
	state.EventScores.ActiveEventID = 1
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for b.Loop() {
		for id := int64(1); id <= 100; id++ {
			if _, found := state.LookupScalableEventScore(id); !found {
				b.Fatal("missing score")
			}
			if !state.ScalableEventScoreReached(id, 1) {
				b.Fatal("score threshold")
			}
			state.EventAvailable(id, now)
		}
	}
}
