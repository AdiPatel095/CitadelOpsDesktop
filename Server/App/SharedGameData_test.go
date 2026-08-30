package App

import (
	"context"
	"path/filepath"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Session"
)

func TestApplicationsShareInjectedGameDataWithoutOwningRefresh(t *testing.T) {
	shared := GameData.NewManager(GameData.UpdaterConfig{
		CacheDir: filepath.Join(t.TempDir(), "shared-game-data"),
	})

	first := newSharedGameDataTestApplication(t, shared)
	second := newSharedGameDataTestApplication(t, shared)

	if first.GameData != shared || second.GameData != shared || first.GameData != second.GameData {
		t.Fatal("account applications did not retain the process-owned game-data manager")
	}
	if first.ownsGameData || second.ownsGameData {
		t.Fatal("an account with injected game data would start a duplicate refresh worker")
	}
}

func TestDesktopApplicationOwnsItsGameData(t *testing.T) {
	application, err := New(context.Background(), Config{
		DataDir:   t.TempDir(),
		Offline:   true,
		Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupApplicationForTest(t, application)
	if application.GameData == nil || !application.ownsGameData {
		t.Fatal("the desktop N=1 application did not retain its standalone game-data lifecycle")
	}
}

func newSharedGameDataTestApplication(t *testing.T, shared *GameData.Manager) *Application {
	t.Helper()
	application, err := New(context.Background(), Config{
		DataDir:   t.TempDir(),
		Offline:   true,
		GameData:  shared,
		Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupApplicationForTest(t, application)
	return application
}

func cleanupApplicationForTest(t *testing.T, application *Application) {
	t.Helper()
	t.Cleanup(func() {
		application.Telemetry.Close()
		_ = application.OperationStore.Close()
		_ = application.ReportStore.Close()
		_ = application.ProfileLease.Close()
	})
}
