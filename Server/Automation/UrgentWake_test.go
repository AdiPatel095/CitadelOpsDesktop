package Automation

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type urgentWakeTestPolicy struct {
	id       string
	domains  []string
	urgent   []string
	requests chan time.Time
}

func (policy *urgentWakeTestPolicy) ID() string { return policy.id }

func (policy *urgentWakeTestPolicy) EnabledKey() string { return policy.id }

func (policy *urgentWakeTestPolicy) WakeDomains() []string { return policy.domains }

func (policy *urgentWakeTestPolicy) UrgentWakeDomains() []string { return policy.urgent }

func (policy *urgentWakeTestPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	if snapshot.State.Khan.PlayerRage < snapshot.State.Khan.PlayerRageCap {
		return Decision{Status: "idle", Detail: "Watching rage", NextCheckAt: snapshot.Now.Add(time.Hour)}, nil
	}
	return Decision{
		Status: "taunting", Detail: "Rage is full", NextCheckAt: snapshot.Now.Add(time.Hour),
		Request: &Intent.Request{Name: "khan.taunt"},
	}, nil
}

type urgentWakeTestSubmitter struct {
	observed chan time.Time
}

func (submitter *urgentWakeTestSubmitter) Submit(context.Context, Intent.Request) Intent.Receipt {
	select {
	case submitter.observed <- time.Now():
	default:
	}
	return Intent.Receipt{Status: Intent.StatusSucceeded}
}

// A full rage bar opens a short game-side window, so the coordinator must not
// spend it inside the shared state-change coalescing delay.
func TestCoordinatorUrgentDomainSkipsStateChangeDebounce(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		urgent  []string
		atMost  time.Duration
		atLeast time.Duration
	}{
		{name: "urgent", urgent: []string{"khan"}, atMost: stateChangeDebounce / 2},
		{name: "coalesced", urgent: nil, atLeast: stateChangeDebounce},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := coordinatorReadyState()
			gameState.Khan.PlayerRageCap = 100
			state := State.NewStore(gameState)
			configuration := openCoordinatorTestConfiguration(t, "rage")
			policy := &urgentWakeTestPolicy{id: "rage", domains: []string{"khan"}, urgent: testCase.urgent}
			submitter := &urgentWakeTestSubmitter{observed: make(chan time.Time, 1)}
			coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				coordinator.Run(ctx)
				close(done)
			}()
			defer func() {
				cancel()
				<-done
			}()

			// Let the initial evaluation settle so the measured wake is the one
			// carrying the full rage bar rather than coordinator startup.
			waitForAutomationStatus(t, state, "rage", "idle")

			filled := time.Now()
			if _, err := state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
				gameState.Khan.PlayerRage = gameState.Khan.PlayerRageCap
				gameState.Khan.RageObservedAt = time.Now().UTC()
				return []string{"khan"}, true, nil
			}); err != nil {
				t.Fatal(err)
			}

			select {
			case observed := <-submitter.observed:
				elapsed := observed.Sub(filled)
				if testCase.atMost > 0 && elapsed > testCase.atMost {
					t.Fatalf("taunt reached the intent engine after %s, want at most %s", elapsed, testCase.atMost)
				}
				if testCase.atLeast > 0 && elapsed < testCase.atLeast {
					t.Fatalf("taunt bypassed coalescing after %s, want at least %s", elapsed, testCase.atLeast)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the full rage bar never reached the intent engine")
			}
		})
	}
}

func waitForAutomationStatus(t *testing.T, state *State.Store, id string, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if state.Snapshot().Automations[id].Status == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("automation %s never reached status %q", id, status)
}

func TestIndexPolicyUrgentWakeDomainsRequiresADeclaredWakeDomain(t *testing.T) {
	indexed := indexPolicyUrgentWakeDomains([]Policy{
		&urgentWakeTestPolicy{id: "rage", domains: []string{"khan"}, urgent: []string{"KHAN ", "map", "session", ""}},
	})
	if got := indexed["khan"]; len(got) != 1 || got[0] != "rage" {
		t.Fatalf("khan urgent index = %v", got)
	}
	if _, promoted := indexed["map"]; promoted {
		t.Fatal("an urgent domain outside WakeDomains was indexed")
	}
	if _, promoted := indexed["session"]; promoted {
		t.Fatal("the session domain was promoted to an urgent wake")
	}
}
