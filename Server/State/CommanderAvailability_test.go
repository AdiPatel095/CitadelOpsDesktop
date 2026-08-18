package State

import (
	"testing"
	"time"
)

func stamp(value string) time.Time {
	parsed, err := time.Parse("2006-01-02 15:04:05.000000", value)
	if err != nil {
		panic(err)
	}
	return parsed.UTC()
}

// returnMovement models a gam listing of a return leg (D=1) exactly as the
// reducer builds it: StartedAt = observed − PT, ReturnsAt = StartedAt + TT.
func returnMovement(observed string, progress, travel int) MovementState {
	observedAt := stamp(observed)
	startedAt := observedAt.Add(-time.Duration(progress) * time.Second)
	returnsAt := startedAt.Add(time.Duration(travel) * time.Second)
	commander := CommanderID(16)
	return MovementState{
		ID: 1, Direction: 1, CommanderID: &commander, ProgressSeconds: progress, TravelSeconds: travel,
		StartedAt: startedAt, ReturnsAt: &returnsAt, ObservedAt: observedAt,
	}
}

// The three CRA 256 rejections Amos took on 2026-08-18 (cell wire log): each
// launch went out 0.1–0.3 s after the return leg's extrapolated end — once
// with the game still listing the movement at PT == TT — and the game
// answered "commander already assigned to an active movement". A commander
// must not be considered free that soon.
func TestCommanderStaysBusyThroughTheGamesReturnSettle(t *testing.T) {
	cases := []struct {
		name     string
		movement MovementState
		launch   string
	}{
		// last gam 13:00:03.49 PT=62 TT=63 → nominal return 13:00:04.49; launch 13:00:04.64
		{"lord 37 storm", returnMovement("2026-08-18 13:00:03.494749", 62, 63), "2026-08-18 13:00:04.643837"},
		// last gam 16:17:39.39 PT=63 TT=66 → nominal 16:17:42.39; launch 16:17:42.68
		{"lord 33 storm", returnMovement("2026-08-18 16:17:39.386731", 63, 66), "2026-08-18 16:17:42.682307"},
		// last gam 19:30:00.64 PT=112 TT=112 (listed at completion); launch 19:30:00.76
		{"lord 16 kingdom 0", returnMovement("2026-08-18 19:30:00.636652", 112, 112), "2026-08-18 19:30:00.761828"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !CommanderMovementActiveAt(testCase.movement, stamp(testCase.launch)) {
				t.Fatalf("commander released at %s, before the game finalised the return", testCase.launch)
			}
			release := CommanderMovementReleaseAt(testCase.movement)
			if release == nil {
				t.Fatal("no release time")
			}
			nominal := testCase.movement.ReturnsAt.UTC()
			if release.Before(nominal.Add(CommanderMovementReturnGrace)) {
				t.Fatalf("release %s is inside the settle window after nominal return %s", release, nominal)
			}
			// Free once the settle window has passed with no later sighting.
			if CommanderMovementActiveAt(testCase.movement, release.Add(time.Millisecond)) {
				t.Fatalf("commander still busy after release %s", release)
			}
		})
	}
}

func TestCommanderReleaseFollowsLaterSightingsPastTheNominalEnd(t *testing.T) {
	// Return leg observed early: PT=10 of TT=100 at 12:00:00 → nominal end
	// 12:01:30, release 12:01:35.
	movement := returnMovement("2026-08-18 12:00:00.000000", 10, 100)
	nominal := movement.ReturnsAt.UTC()
	if release := CommanderMovementReleaseAt(movement); !release.Equal(nominal.Add(CommanderMovementReturnGrace)) {
		t.Fatalf("release = %s, want nominal + grace = %s", release, nominal.Add(CommanderMovementReturnGrace))
	}
	// The game still lists it 3 s after the nominal end: release moves to that
	// sighting + grace, so the commander stays busy until a later reply drops
	// the movement.
	movement.ObservedAt = nominal.Add(3 * time.Second)
	movement.ProgressSeconds = 100
	if release := CommanderMovementReleaseAt(movement); !release.Equal(nominal.Add(8 * time.Second)) {
		t.Fatalf("release after late sighting = %s, want nominal + 8s", release)
	}
	if !CommanderMovementActiveAt(movement, nominal.Add(7*time.Second)) {
		t.Fatal("commander freed while the game still listed the movement")
	}
	if CommanderMovementActiveAt(movement, nominal.Add(8*time.Second+time.Millisecond)) {
		t.Fatal("a stalled poll must not pin the commander past the settle window")
	}
}

func TestOutboundLegReleaseCoversTheReturnTripPlusGrace(t *testing.T) {
	arrives := stamp("2026-08-18 12:00:00.000000")
	commander := CommanderID(7)
	movement := MovementState{
		ID: 2, Direction: 0, CommanderID: &commander, TravelSeconds: 300, ArrivesAt: &arrives,
		ObservedAt: arrives.Add(-200 * time.Second),
	}
	want := arrives.Add(300 * time.Second).Add(CommanderMovementReturnGrace)
	if release := CommanderMovementReleaseAt(movement); release == nil || !release.Equal(want) {
		t.Fatalf("outbound release = %v, want %s", release, want)
	}
	station := MovementState{ID: 3, Direction: 0, WaitSeconds: 60, CommanderID: &commander, ArrivesAt: &arrives}
	if CommanderMovementReleaseAt(station) != nil || !CommanderMovementActiveAt(station, arrives.Add(time.Hour)) {
		t.Fatal("a stationed movement holds its commander until the game says otherwise")
	}
}
