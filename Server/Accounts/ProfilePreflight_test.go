package Accounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceAdmissionRejectsOversizedProfileBeforeFenceOrStop(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	app, _ := s.Application("alpha")
	file, err := os.Create(filepath.Join(app.DataDir, "oversized-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(16 << 30); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if err := o.preflightSourceProfile(t.Context(), identity); err == nil {
		t.Fatal("oversized live profile admitted")
	}
	if _, err := o.PrepareSourceHandover(t.Context(), identity); err == nil {
		t.Fatal("oversized profile stopped")
	}
	if current, exists := s.Application("alpha"); !exists || current != app {
		t.Fatal("source changed after failed admission")
	}
	if _, exists := s.sourceFence("alpha"); exists {
		t.Fatal("failed admission fenced source")
	}
}
