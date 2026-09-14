package Accounts

import (
	"testing"
	"time"
)

func TestHandoverPreservedRuntimeIsNotRestartedOrDrained(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	o.handoverTransport = true
	original, _ := s.Application("alpha")
	sibling := testAssignment("sibling", "tenant-one", 1, o.now().Add(time.Minute))
	desired := ReconcileRequest{SchemaVersion: 1, Revision: o.Status().DesiredRevision + 1, Runtimes: []RuntimeAssignment{sibling}, PreserveRuntimes: []string{"alpha", "absent"}}
	if _, err := o.Reconcile(t.Context(), desired); err != nil {
		t.Fatal(err)
	}
	if current, exists := s.Application("alpha"); !exists || current != original {
		t.Fatal("preserved App changed")
	}
	if _, exists := s.Application("absent"); exists {
		t.Fatal("absent preserved runtime was created")
	}
	if _, err := o.ExportSourceProfile(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	desired.Revision++
	if _, err := o.Reconcile(t.Context(), desired); err != nil {
		t.Fatal(err)
	}
	if _, exists := s.Application("alpha"); exists {
		t.Fatal("retired source was restarted")
	}
	if _, exists := s.Application("sibling"); !exists {
		t.Fatal("sibling was drained")
	}
	desired.Revision++
	desired.Runtimes = nil
	if _, err := o.Reconcile(t.Context(), desired); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, s, "sibling", false)
}

func TestHandoverPreserveRejectsAmbiguousOrDisabledRequests(t *testing.T) {
	_, _, o, _, _ := handoverSourceFixture(t)
	for _, ids := range [][]string{{"alpha"}, {"../alpha"}, {"alpha", "alpha"}} {
		request := ReconcileRequest{SchemaVersion: 1, Revision: 2, PreserveRuntimes: ids}
		if _, err := o.Reconcile(t.Context(), request); err == nil {
			t.Fatal("disabled preserve accepted")
		}
	}
	o.handoverTransport = true
	assignment := testAssignment("alpha", "tenant-one", 4, o.now().Add(time.Minute))
	if _, err := o.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: []RuntimeAssignment{assignment}, PreserveRuntimes: []string{"alpha"}}); err == nil {
		t.Fatal("preserve and assign accepted")
	}
}
