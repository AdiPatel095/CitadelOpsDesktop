package Session

import (
	"context"
	"testing"
	"time"
)

func TestLoginCooldownDeadlines(t *testing.T) {
	observed := time.Date(2026, 9, 22, 1, 2, 3, 0, time.UTC)
	for _, tc := range []struct {
		name, payload  string
		fallback, want time.Duration
		valid          bool
	}{
		{"positive", `{"CD":10}`, 5 * time.Minute, 12 * time.Second, true},
		{"zero", `{"CD":0}`, 5 * time.Minute, 2 * time.Second, true},
		{"missing", `{}`, 7 * time.Second, 7 * time.Second, false},
		{"null", `{"CD":null}`, 0, 2 * time.Second, false},
		{"negative", `{"CD":-5}`, 0, 2 * time.Second, false},
		{"malformed", `{"CD":`, 0, 2 * time.Second, false},
		{"string", `{"CD":"4"}`, 0, 2 * time.Second, false},
		{"fraction", `{"CD":0.5}`, 0, 2 * time.Second, false},
		{"overflow", `{"CD":9223372036854775807}`, 0, 2 * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, retry := loginCooldownDeadlines([]byte(tc.payload), observed, tc.fallback)
			if (until != nil) != tc.valid || !retry.Equal(observed.Add(tc.want)) {
				t.Fatalf("until=%v retry=%v", until, retry)
			}
		})
	}
}

func TestChromiumCooldownRetryFences(t *testing.T) {
	for _, mode := range []string{"due", "superseded", "success", "stopped", "generation", "connection", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			tr := newSocketTestTransport()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tr.gameContext = ctx
			reloads := 0
			tr.reloadEvaluator = func(context.Context) error { reloads++; return nil }
			// Keep the automatically scheduled timer far in the future; exercise its
			// expiry synchronously below, without browser or wall-clock sleeps.
			observed := time.Now().Add(time.Hour)
			tr.observeLoginFrame(1, "", `%xt%lli%1%453%{"CD":0}%`, observed)
			first := *tr.Status().RetryAt
			switch mode {
			case "superseded":
				tr.observeLoginFrame(1, "", `%xt%lli%1%453%{"CD":20}%`, observed.Add(time.Second))
			case "success":
				tr.observeLoginFrame(1, "", `%xt%lli%1%0%{}%`, observed.Add(time.Second))
			case "stopped":
				tr.cancel = nil
			case "generation":
				tr.generation++
			case "connection":
				tr.status.ConnectionGeneration++
			case "cancelled":
				cancel()
			}
			tr.reloadAfter(1, 0, 0, first)
			want := 0
			if mode == "due" {
				want = 1
			}
			if reloads != want {
				t.Fatalf("reloads=%d want=%d", reloads, want)
			}
		})
	}
}

func TestReleasedCooldownUsesResponseReceipt(t *testing.T) {
	observed := time.Now().UTC().Add(-time.Second)
	tr := &DirectWebSocketTransport{runGeneration: 1, cancel: func() {}, statuses: make(chan Status, 1)}
	tr.release(1, Status{}, &directLoginError{code: 453, cooldownPayload: []byte(`{"CD":10}`), cooldownObservedAt: observed}, true, 5*time.Minute, time.Now().UTC())
	status := tr.Status()
	if status.State != "released" || status.RetryAt == nil || !status.RetryAt.Equal(observed.Add(12*time.Second)) {
		t.Fatalf("status=%+v", status)
	}
}
