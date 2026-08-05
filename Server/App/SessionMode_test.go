package App

import (
	"context"
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Session"
)

func TestApplicationSelectsConfiguredSessionTransportAtStartup(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		want Session.ConnectionMode
	}{
		{name: "full application is the default", mode: "full", want: Session.ConnectionModeFull},
		{name: "background only", mode: "background", want: Session.ConnectionModeBackground},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			configuration, err := Configuration.Open(dataDir, defaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := configuration.Update(
				"session.connection", json.RawMessage(`{"mode":"`+test.mode+`"}`),
			); err != nil {
				t.Fatal(err)
			}
			application, err := New(context.Background(), Config{
				DataDir: dataDir, Offline: true, Chromium: &Session.ChromiumConfig{},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				application.Telemetry.Close()
				_ = application.OperationStore.Close()
				_ = application.ReportStore.Close()
				_ = application.ProfileLease.Close()
			})
			if got := application.Session.Status().Mode; got != test.want {
				t.Fatalf("startup session mode = %q, want %q", got, test.want)
			}
		})
	}
}
