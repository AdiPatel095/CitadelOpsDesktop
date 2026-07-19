package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type kingdomResourceShipmentGood struct {
	ResourceID State.ResourceID `json:"resourceId"`
	Amount     int64            `json:"amount"`
}

type kingdomResourceShipmentRequest struct {
	SourceCastleID  State.CastleID                `json:"sourceCastleId"`
	TargetCastleID  State.CastleID                `json:"targetCastleId,omitempty"`
	TargetKingdomID State.KingdomID               `json:"targetKingdomId"`
	ResourceID      State.ResourceID              `json:"resourceId,omitempty"`
	Amount          int64                         `json:"amount,omitempty"`
	Goods           []kingdomResourceShipmentGood `json:"goods,omitempty"`
}

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
	hasMarketplace, err := input.GameData.CastleHasMarketplace(source)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("check marketplace at %s: %w", castleLabel(source), err)
	}
	if !hasMarketplace {
		return Intent.Plan{}, fmt.Errorf("source castle %d has no Marketplace building", source.ID)
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
	var request kingdomResourceShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	goods, err := normalizeKingdomResourceGoods(request)
	if err != nil {
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
	var target State.CastleState
	if request.TargetCastleID > 0 {
		var targetExists bool
		target, targetExists = input.State.Castles[request.TargetCastleID]
		if !targetExists || target.KingdomID != request.TargetKingdomID {
			return Intent.Plan{}, fmt.Errorf("target castle %d is not in kingdom %d", request.TargetCastleID, request.TargetKingdomID)
		}
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
	wireGoods := make([][]any, 0, len(goods))
	summaryGoods := make([]string, 0, len(goods))
	for _, good := range goods {
		resourceKey, resourceErr := officialResourceJSONKey(input.GameData, good.ResourceID)
		if resourceErr != nil {
			return Intent.Plan{}, resourceErr
		}
		if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
			return Intent.Plan{}, fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceKey)
		}
		wireGoods = append(wireGoods, []any{resourceKey, good.Amount})
		summaryGoods = append(summaryGoods, fmt.Sprintf("%d %s", good.Amount, resourceKey))
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		Goods          [][]any         `json:"G"`
	}{source.ID, source.KingdomID, request.TargetKingdomID, wireGoods})
	summary := fmt.Sprintf("Ship %s from kingdom %d to %d", strings.Join(summaryGoods, " and "), source.KingdomID, request.TargetKingdomID)
	if request.TargetCastleID > 0 {
		summary = fmt.Sprintf("Ship %s from %s to %s by kingdom transport", strings.Join(summaryGoods, " and "), castleLabel(source), castleLabel(target))
	}
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
		},
		Summary: summary,
		Steps: []Intent.Step{
			kingdomTransportContextStep(),
			commandStep("Start kingdom resource shipment", "kgt", payload, "kgt"),
		},
	}, nil
}

func normalizeKingdomResourceGoods(request kingdomResourceShipmentRequest) ([]kingdomResourceShipmentGood, error) {
	if len(request.Goods) > 0 && (request.ResourceID != 0 || request.Amount != 0) {
		return nil, fmt.Errorf("use either goods or the legacy resourceId and amount fields, not both")
	}
	goods := append([]kingdomResourceShipmentGood(nil), request.Goods...)
	if len(goods) == 0 {
		goods = []kingdomResourceShipmentGood{{ResourceID: request.ResourceID, Amount: request.Amount}}
	}
	if len(goods) > 20 {
		return nil, fmt.Errorf("goods may contain at most 20 resources")
	}
	merged := map[State.ResourceID]int64{}
	for _, good := range goods {
		if good.ResourceID <= 0 || good.Amount <= 0 {
			return nil, fmt.Errorf("every shipment good requires a positive resourceId and amount")
		}
		if merged[good.ResourceID] > int64(^uint64(0)>>1)-good.Amount {
			return nil, fmt.Errorf("shipment amount for resource %d is too large", good.ResourceID)
		}
		merged[good.ResourceID] += good.Amount
	}
	result := make([]kingdomResourceShipmentGood, 0, len(merged))
	for resourceID, amount := range merged {
		result = append(result, kingdomResourceShipmentGood{ResourceID: resourceID, Amount: amount})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ResourceID < result[right].ResourceID })
	return result, nil
}

func planKingdomResourceSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
		TimeSkipID       string          `json:"timeSkipId"`
		MinimumRemaining int64           `json:"minimumRemaining,omitempty"`
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
	if request.MinimumRemaining < 0 {
		return Intent.Plan{}, fmt.Errorf("minimumRemaining cannot be negative")
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
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
