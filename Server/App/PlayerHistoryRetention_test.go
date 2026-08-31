package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func TestCapturePlayerHistoryConfigurationEventClearsAndNoneSuppressesRecapture(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(History.CollectionPlayerSamples, History.PlayerSample{
		TimestampUnix: time.Now().UTC().Unix(), PlayerID: 42, Might: 100,
	}); err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Player.ID = 42
	state.Player.Might = 100
	stateStore := State.NewStore(state)
	application := &Application{
		State:         stateStore,
		Configuration: configuration,
		History:       history,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		application.capturePlayerHistory(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("player history capture did not stop")
		}
	})

	if _, err := configuration.Update(
		History.PlayerSamplesConfigurationSection,
		json.RawMessage(`{"version":1,"retention":"none"}`),
	); err != nil {
		t.Fatal(err)
	}
	waitForPlayerSamplesCount(t, history, 42, 0, 2*time.Second)

	if _, err := stateStore.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Might++
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	// A first state event would normally schedule a capture after 250ms. Waiting
	// beyond that boundary proves the saved "none" policy suppresses re-capture.
	time.Sleep(500 * time.Millisecond)
	samples, err := history.PlayerSamplesForPlayer(time.Time{}, 100, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatalf("none retention re-captured player history: %+v", samples)
	}
}

func TestCapturePlayerHistoryConfigurationEventAppliesNewRecordingCadence(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"unlimited","recordingIntervalSeconds":60}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(15 * time.Minute).Add(-30 * time.Minute)
	for minute := 0; minute < 30; minute++ {
		if err := history.Append(History.CollectionPlayerSamples, History.PlayerSample{
			TimestampUnix: start.Add(time.Duration(minute) * time.Minute).Unix(),
			PlayerID:      42,
			Might:         float64(minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	application := &Application{
		State:         State.NewStore(State.NewGameState()),
		Configuration: configuration,
		History:       history,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		application.capturePlayerHistory(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("player history capture did not stop")
		}
	}()

	if _, err := configuration.Update(
		History.PlayerSamplesConfigurationSection,
		json.RawMessage(`{"version":1,"retention":"unlimited","recordingIntervalSeconds":900}`),
	); err != nil {
		t.Fatal(err)
	}
	waitForPlayerSamplesCount(t, history, 42, 2, 2*time.Second)
}

func waitForPlayerSamplesCount(
	t *testing.T,
	history *History.Store,
	playerID State.PlayerID,
	want int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		samples, err := history.PlayerSamplesForPlayer(time.Time{}, 100, playerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("player sample count = %d, want %d", len(samples), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
