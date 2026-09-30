package State

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestMapCoordinateKeyMatchesSprintf(t *testing.T) {
	values := []int{0, 1, -1, 999, -999, math.MaxInt32, math.MinInt32, math.MaxInt, math.MinInt}
	for _, x := range values {
		for _, y := range values {
			if got, want := MapCoordinateKey(x, y), fmt.Sprintf("%d:%d", x, y); got != want {
				t.Fatal(got, want)
			}
		}
	}
}
func TestMapChangeKeyMatchesSprintf(t *testing.T) {
	for _, k := range []KingdomID{0, 1, -1, 1000000000000000000, -1000000000000000000, math.MaxInt64, math.MinInt64} {
		for _, key := range []string{"", "1:2", "-999:999", "Fixture"} {
			if got, want := mapChangeKey(k, key), fmt.Sprintf("%020d:%s", k, key); got != want {
				t.Fatal(got, want)
			}
		}
	}
}
func TestSetMapObservationComparesByValue(t *testing.T) {
	s := NewGameState()
	o := MapObservation{KingdomID: 0, TypeID: MapTypeKingdomTower, X: 100, Y: 101, ObservedAt: time.Now().UTC()}
	if !s.SetMapObservation(o) {
		t.Fatal("first")
	}
	changes := len(s.mapChanges())
	if s.SetMapObservation(o) || len(s.mapChanges()) != changes {
		t.Fatal("same changed")
	}
	o.ObservedAt = o.ObservedAt.Add(time.Nanosecond)
	if !s.SetMapObservation(o) {
		t.Fatal("time ignored")
	}
}
