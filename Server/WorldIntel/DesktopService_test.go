package WorldIntel

import (
	"context"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestDesktopServiceReportsSharedDataReaderMode(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Account.WorldID = "https://WORLD.EXAMPLE/socket"
	service := NewDesktopService(
		State.NewStore(gameState),
		NewCloudClient(ClientConfig{BaseURL: "https://intel.example/v1", ClientVersion: "test"}),
	)
	status := service.Status(context.Background())
	if !status.Enabled || status.CollectionMode != "shared-data-reader" || status.WorldID != "world.example" ||
		status.Endpoint != "https://intel.example/v1" || !status.PublicFieldsOnly || !status.OfficialSourceOnly {
		t.Fatalf("unexpected read-only status: %#v", status)
	}
}
