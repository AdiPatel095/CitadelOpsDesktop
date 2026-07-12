package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func planResourceLogisticsRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	return Intent.Plan{
		Claims: []string{"resource-transport"}, Summary: "Refresh resource logistics state",
		Steps: []Intent.Step{
			commandStep("Refresh kingdom transports", "kpi", json.RawMessage(`{}`), "kpi"),
			commandStep("Refresh caravan boosters", "boi", json.RawMessage(`{}`), "boi"),
			commandStep("Refresh market capacity", "cmi", json.RawMessage(`{"S":1,"KID":-1}`), "cmi"),
		},
	}, nil
}

func planMarketResourceShipment(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if !targetExists || request.TargetCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("target castle %d is not in the current player state", request.TargetCastleID)
	}
	if source.ID == target.ID || source.KingdomID != target.KingdomID {
		return Intent.Plan{}, fmt.Errorf("market shipments require distinct castles in the same kingdom")
	}
	resourceKey, err := officialResourceJSONKey(input.GameData, request.ResourceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("amount must be positive")
	}
	if source.Resources[request.ResourceID].Amount < float64(request.Amount) {
		return Intent.Plan{}, fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceKey)
	}
	market, observed := input.State.Market.Castles[source.ID]
	if !observed || input.State.Market.ObservedAt.IsZero() || market.AvailableBarrows <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d has no observed available market barrows", source.ID)
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		SourceID  State.CastleID  `json:"SID"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		HorseWID  int             `json:"HBW"`
		Goods     [][]any         `json:"G"`
		PaidTime  int             `json:"PTT"`
		DelaySec  int             `json:"SD"`
	}{
		KingdomID: source.KingdomID, SourceID: source.ID, TargetX: target.X, TargetY: target.Y,
		HorseWID: -1, Goods: [][]any{{resourceKey, request.Amount}},
	})
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"castle:" + strconv.FormatInt(int64(target.ID), 10),
		},
		Summary: fmt.Sprintf("Ship %d %s from %s to %s", request.Amount, resourceKey, castleLabel(source), castleLabel(target)),
		Steps:   []Intent.Step{commandStep("Start market shipment", "crm", payload, "crm")},
	}, nil
}

func planKingdomResourceShipment(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID  State.CastleID   `json:"sourceCastleId"`
		TargetKingdomID State.KingdomID  `json:"targetKingdomId"`
		ResourceID      State.ResourceID `json:"resourceId"`
		Amount          int64            `json:"amount"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if request.TargetKingdomID < 0 || request.TargetKingdomID == source.KingdomID {
		return Intent.Plan{}, fmt.Errorf("targetKingdomId must identify a different owned kingdom")
	}
	targetOwned := false
	for _, castle := range input.State.Castles {
		if castle.KingdomID == request.TargetKingdomID {
			targetOwned = true
			break
		}
	}
	if !targetOwned {
		return Intent.Plan{}, fmt.Errorf("kingdom %d has no castle in the current player state", request.TargetKingdomID)
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[request.TargetKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked {
		return Intent.Plan{}, fmt.Errorf("kingdom transport to %d is not observed as unlocked", request.TargetKingdomID)
	}
	for _, pending := range input.State.KingdomTransport.Pending {
		if pending.KingdomID == request.TargetKingdomID && pending.RemainingSec > 0 {
			return Intent.Plan{}, fmt.Errorf("kingdom %d already has a pending resource transport", request.TargetKingdomID)
		}
	}
	resourceKey, err := officialResourceJSONKey(input.GameData, request.ResourceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("amount must be positive")
	}
	if source.Resources[request.ResourceID].Amount < float64(request.Amount) {
		return Intent.Plan{}, fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceKey)
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		Goods          [][]any         `json:"G"`
	}{source.ID, source.KingdomID, request.TargetKingdomID, [][]any{{resourceKey, request.Amount}}})
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
		},
		Summary: fmt.Sprintf("Ship %d %s from kingdom %d to %d", request.Amount, resourceKey, source.KingdomID, request.TargetKingdomID),
		Steps:   []Intent.Step{commandStep("Start kingdom resource shipment", "kgt", payload, "kgt")},
	}, nil
}

func planKingdomResourceSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		TargetKingdomID State.KingdomID `json:"targetKingdomId"`
		TimeSkipID      string          `json:"timeSkipId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Plan{}, fmt.Errorf("targetKingdomId and timeSkipId are required")
	}
	pending := false
	for _, transport := range input.State.KingdomTransport.Pending {
		if transport.KingdomID == request.TargetKingdomID && transport.RemainingSec > 0 {
			pending = true
			break
		}
	}
	if !pending {
		return Intent.Plan{}, fmt.Errorf("kingdom %d has no pending resource transport", request.TargetKingdomID)
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.State.Player.Currencies[currencyID] < 1 {
		return Intent.Plan{}, fmt.Errorf("no %s time skips are available", request.TimeSkipID)
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "2",
	})
	return Intent.Plan{
		Claims:  []string{"resource-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10)},
		Summary: fmt.Sprintf("Apply %s to kingdom %d resource transport", request.TimeSkipID, request.TargetKingdomID),
		Steps:   []Intent.Step{commandStep("Skip kingdom resource transport time", "msk", payload, "msk")},
	}, nil
}

func officialResourceJSONKey(store *GameData.Store, id State.ResourceID) (string, error) {
	if store == nil || id <= 0 {
		return "", fmt.Errorf("resourceId must reference the loaded official catalog")
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return "", err
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(id), 10))
	if !exists {
		return "", fmt.Errorf("resource %d is not in the current official catalog", id)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return "", err
	}
	jsonKey, _ := record.String("JSONKey")
	jsonKey = strings.TrimSpace(jsonKey)
	if jsonKey == "" {
		return "", fmt.Errorf("resource %d has no official wire key", id)
	}
	return jsonKey, nil
}

func officialCurrencyID(store *GameData.Store, jsonKey string) (State.CurrencyID, error) {
	if store == nil {
		return 0, fmt.Errorf("official currency catalog is unavailable")
	}
	catalog, err := store.Catalog("currencies")
	if err != nil {
		return 0, err
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, _ := record.String("JSONKey")
		if !strings.EqualFold(strings.TrimSpace(candidate), jsonKey) {
			continue
		}
		id, _ := record.Int64("currencyID")
		if id > 0 {
			return State.CurrencyID(id), nil
		}
	}
	return 0, fmt.Errorf("currency %s is not in the current official catalog", jsonKey)
}
