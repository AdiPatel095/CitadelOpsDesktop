package Session

import (
	"testing"
	"time"
)

func TestLoginCooldownPublishesCountdownDeadlines(t *testing.T) {
	transport := &ChromiumTransport{statuses: make(chan Status, 1), status: Status{Namespace: "EmpireEx_21"}}
	before := time.Now().UTC()
	transport.observeLoginFrame(0, `%xt%lli%1%453%{"CD":10}%`)
	status := transport.Status()
	if status.State != "cooldown" || status.CooldownUntil == nil || status.RetryAt == nil {
		t.Fatalf("unexpected cooldown status: %+v", status)
	}
	if status.CooldownUntil.Before(before.Add(9*time.Second)) || status.RetryAt.Sub(*status.CooldownUntil) != 5*time.Second {
		t.Fatalf("unexpected cooldown deadlines: %+v", status)
	}
}
