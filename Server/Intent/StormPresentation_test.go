package Intent

import (
	"CitadelDesktop/Server/Localization"
	"testing"
)

func TestStormStatusListsRemainAtRoot(t *testing.T) {
	action := Localization.WithLists(Localization.New("server.storm.purchase_plan", "Buy {purchases} from Luna for {cost, number} Aquamarine at {castle}", Localization.Params{"cost": 12, "castle": "literal {castle}"}), "original action", map[string][]*Localization.Message{"purchases": {Localization.New("item", "Package {id}", Localization.Params{"id": "245"})}})
	explanation := Localization.New("reason", "Exact reason", nil)
	for _, status := range []string{"completed", "completed_batch", "failed", "partial", "unconfirmed", "failed_activity"} {
		got := StormPlanStatusDescriptor(action, status, "complete original status", explanation, 2, 3)
		if got == nil || got.FallbackText != "complete original status" || len(got.Context) != 0 || len(got.ListParams["purchases"]) != 1 {
			t.Fatalf("%s: %#v", status, got)
		}
		if err := Localization.Validate(got); err != nil {
			t.Fatal(err)
		}
	}
	for _, reason := range []*Localization.Message{nil, {Key: "reason", Fallback: "Reason", Context: []*Localization.Message{explanation}}, action} {
		if StormPlanStatusDescriptor(action, "failed_activity", "raw", reason, 0, 0) != nil {
			t.Fatal("unknown or nested explanation concealed")
		}
	}
	if len(action.ListParams) != 1 {
		t.Fatal("status mutated plan")
	}
	for _, status := range []Status{StatusFailed, StatusPartiallySucceeded, StatusIndeterminate} {
		receipt := Receipt{Status: status, Plan: &Plan{Summary: "original action", SummaryDescriptor: action}}
		got := failureHeadlineDescriptor(receipt)
		if got == nil || got.FallbackText != failureHeadline(receipt) || got.ListParams == nil {
			t.Fatalf("headline %s: %#v", status, got)
		}
	}
}
