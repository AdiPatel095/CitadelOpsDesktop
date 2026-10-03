package Ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

// testdata/gam_quiet_account_replay.jsonl is a synthetic, sanitized capture of a
// quiet account's gam polls: 36 replies about 5 s apart (with arrival jitter).
// It contains no real player, alliance or movement identifiers. Frames 0-11 show
// two owned movements (an attack with a commander and a long station wait), an
// incoming attack appears at frame 12 and lands at frame 24; every other frame is
// the same situation with only the elapsed-travel counters advanced.
func TestReplayOfAQuietAccountCommitsOnlyRealMovementChanges(t *testing.T) {
	file, err := os.Open("testdata/gam_quiet_account_replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	pipeline, store := quietPollFixture(t)
	events, unsubscribe := store.Subscribe(256)
	defer unsubscribe()

	start := movementTestStart()
	var committedAt []int
	frames := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var line struct {
			ReceivedAtOffsetMs int64           `json:"receivedAtOffsetMs"`
			Payload            json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		before := store.Revision()
		code := 0
		committed, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
			Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
			ReceivedAt: start.Add(time.Duration(line.ReceivedAtOffsetMs) * time.Millisecond), Payload: line.Payload,
		})
		if err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		if committed.Revision != before {
			committedAt = append(committedAt, frames)
		}
		frames++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if frames != 36 {
		t.Fatalf("replayed %d frames, want 36", frames)
	}
	if want := []int{0, 12, 24}; len(committedAt) != len(want) || committedAt[0] != want[0] || committedAt[1] != want[1] || committedAt[2] != want[2] {
		t.Fatalf("frames that created a revision = %v, want %v", committedAt, want)
	}
	if got := len(events); got != 3 {
		t.Fatalf("state events = %d, want 3 (the first snapshot, the incoming attack appearing, and it landing)", got)
	}
	final := store.ReadOnlyView()
	if final.MovementCount() != 2 || final.MovementSnapshot.ObservedAt.Sub(start) < 170*time.Second {
		t.Fatalf("final state: %d movements, snapshot observed %s after start", final.MovementCount(), final.MovementSnapshot.ObservedAt.Sub(start))
	}
}
