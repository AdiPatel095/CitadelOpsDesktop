package App

import (
	"context"
	"encoding/json"

	"CitadelDesktop/Server/Outbound"
)

func (application *Application) clearAutomationSafetyLock(ctx context.Context, arguments json.RawMessage) error {
	var request struct {
		Lane        string `json:"lane"`
		OperationID string `json:"operationId"`
		Review      string `json:"review"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	return application.Intents.ClearAutomationLaneLock(request.Lane, request.OperationID, request.Review, Outbound.MetadataFromContext(ctx).Actor)
}
