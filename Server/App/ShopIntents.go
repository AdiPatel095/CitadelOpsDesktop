package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func (application *Application) registerShopIntents() error {
	definitions := []Intent.Definition{
		{
			Name:             "shop.package.history",
			Description:      "Request GBC purchase counters for stock-limited and rebuyable packages",
			Effect:           Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"castleId":12345678}`),
			Planner:          planShopPackageHistory,
		},
		{
			Name:             "shop.package.purchase",
			Description:      "Send an explicit capture-shaped SBP purchase for an official package and shop table",
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"productId":3068,"tableId":116,"amount":10,"kingdomId":0,"castleId":-1,"buildType":0,"premiumCost":-1,"buyAll":0,"power":0,"position":-1}`),
			Planner:          planShopPackagePurchase,
		},
		{
			Name:             "shop.mercenary.refresh",
			Description:      "Request the current Mercenary Post offers with the captured MPE shape",
			Effect:           Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{}`),
			Planner:          planShopMercenaryRefresh,
		},
		{
			Name:             "shop.mercenary.purchase",
			Description:      "Attempt a Mercenary Post slot purchase with the observed MBS shape",
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"slotId":1}`),
			Planner:          planShopMercenaryPurchase,
		},
		{
			Name:             "shop.offer.purchase",
			Description:      "Send the first OOP purchase step; code 440 returns the server-confirmed premium price",
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"offerId":5894,"count":1,"optionIndexes":[0]}`),
			Planner:          planShopOfferPurchase,
		},
		{
			Name:             "shop.offer.confirm",
			Description:      "Confirm an OOP purchase using the exact CC2T value returned by code 440",
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"offerId":5894,"count":1,"optionIndexes":[0],"confirmedPremiumCost":3500}`),
			Planner:          planShopOfferConfirm,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planShopPackageHistory(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID  State.CastleID   `json:"castleId"`
		KingdomID *State.KingdomID `json:"kingdomId,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	kingdomID := castle.KingdomID
	if request.KingdomID != nil {
		kingdomID = *request.KingdomID
		if kingdomID != castle.KingdomID {
			return Intent.Plan{}, fmt.Errorf("castle %d belongs to kingdom %d, not %d", castle.ID, castle.KingdomID, kingdomID)
		}
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, kingdomID})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:purchase-history"},
		Summary: fmt.Sprintf("Load stock-limited package purchase counters for %s", castleLabel(castle)),
		Steps:   []Intent.Step{shopCommandStep("Load package purchase history", "gbc", payload, 0)},
	}, nil
}

func planShopPackagePurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		ProductID State.PackageID  `json:"productId"`
		TableID   int64            `json:"tableId"`
		Amount    int64            `json:"amount"`
		KingdomID *State.KingdomID `json:"kingdomId,omitempty"`
		CastleID  *int64           `json:"castleId,omitempty"`
		BuildType *int64           `json:"buildType,omitempty"`
		Premium   *int64           `json:"premiumCost,omitempty"`
		BuyAll    *int64           `json:"buyAll,omitempty"`
		Power     *int64           `json:"power,omitempty"`
		Position  *int64           `json:"position,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil || request.ProductID <= 0 {
		return Intent.Plan{}, fmt.Errorf("productId must reference the loaded official package catalog")
	}
	packages, err := input.GameData.Catalog("packages")
	if err != nil {
		return Intent.Plan{}, err
	}
	rawPackage, exists := packages.Find(strconv.FormatInt(int64(request.ProductID), 10))
	if !exists {
		return Intent.Plan{}, fmt.Errorf("package %d is not in the current official catalog", request.ProductID)
	}
	packageRecord, err := GameData.DecodeRecord(rawPackage)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("decode package %d: %w", request.ProductID, err)
	}
	if request.TableID <= 0 {
		return Intent.Plan{}, fmt.Errorf("tableId must be positive")
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("amount must be positive")
	}
	if stock, limited := packageRecord.Int64("stock"); limited && stock > 0 {
		purchased := input.State.Inventory.ConstructionOffers[request.ProductID]
		if request.Amount > stock || purchased+request.Amount > stock {
			remaining := max(int64(0), stock-purchased)
			return Intent.Plan{}, fmt.Errorf("amount exceeds package %d remaining stock %d", request.ProductID, remaining)
		}
	}

	kingdomID := int64Value(request.KingdomID, 0)
	castleID := int64Value(request.CastleID, -1)
	buildType := int64Value(request.BuildType, 0)
	premium := int64Value(request.Premium, -1)
	buyAll := int64Value(request.BuyAll, 0)
	power := int64Value(request.Power, 0)
	position := int64Value(request.Position, -1)
	if kingdomID < 0 {
		return Intent.Plan{}, fmt.Errorf("kingdomId cannot be negative")
	}
	if castleID == 0 || castleID < -1 {
		return Intent.Plan{}, fmt.Errorf("castleId must be -1 or a positive castle instance id")
	}
	if premium < -1 {
		return Intent.Plan{}, fmt.Errorf("premiumCost cannot be less than -1")
	}
	if buyAll < 0 || power < 0 || position < -1 {
		return Intent.Plan{}, fmt.Errorf("buyAll and power cannot be negative, and position cannot be less than -1")
	}
	payload, _ := json.Marshal(struct {
		ProductID State.PackageID `json:"PID"`
		BuildType int64           `json:"BT"`
		TableID   int64           `json:"TID"`
		Amount    int64           `json:"AMT"`
		KingdomID int64           `json:"KID"`
		CastleID  int64           `json:"AID"`
		Premium   int64           `json:"PC2"`
		BuyAll    int64           `json:"BA"`
		Power     int64           `json:"PWR"`
		Position  int64           `json:"_PO"`
	}{request.ProductID, buildType, request.TableID, request.Amount, kingdomID, castleID, premium, buyAll, power, position})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:table:" + strconv.FormatInt(request.TableID, 10), "account-resources"},
		Summary: fmt.Sprintf("Purchase official package %d from shop table %d", request.ProductID, request.TableID),
		Steps:   []Intent.Step{shopCommandStep("Purchase shop package", "sbp", payload, 0)},
	}, nil
}

func planShopMercenaryRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims:  []string{"shop", "shop:mercenary"},
		Summary: "Load current Mercenary Post offers",
		Steps:   []Intent.Step{shopCommandStep("Load Mercenary Post", "mpe", json.RawMessage(`{"MID":-1}`), 0)},
	}, nil
}

func planShopMercenaryPurchase(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SlotID int64 `json:"slotId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.SlotID < 1 || request.SlotID > 6 {
		return Intent.Plan{}, fmt.Errorf("slotId must be between 1 and 6")
	}
	payload, _ := json.Marshal(struct {
		SlotID int64 `json:"SID"`
	}{request.SlotID})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:mercenary", "account-resources"},
		Summary: fmt.Sprintf("Purchase Mercenary Post slot %d", request.SlotID),
		Steps:   []Intent.Step{shopCommandStep("Purchase Mercenary Post slot", "mbs", payload, 0)},
	}, nil
}

func planShopOfferPurchase(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := decodeShopOfferRequest(arguments, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(struct {
		OfferID       int64   `json:"OID"`
		Count         int64   `json:"C"`
		OptionIndexes []int64 `json:"ODI"`
	}{request.OfferID, request.Count, request.OptionIndexes})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:offer", "account-resources"},
		Summary: fmt.Sprintf("Submit offer %d purchase and capture its confirmation response", request.OfferID),
		Steps:   []Intent.Step{shopCommandStep("Submit offer purchase", "oop", payload, 0, 440)},
	}, nil
}

func planShopOfferConfirm(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := decodeShopOfferRequest(arguments, true)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(struct {
		CommandID     string  `json:"cmdID"`
		OfferID       int64   `json:"OID"`
		Count         int64   `json:"C"`
		OptionIndexes []int64 `json:"ODI"`
		Premium       int64   `json:"CC2T"`
	}{"oop", request.OfferID, request.Count, request.OptionIndexes, request.ConfirmedPremiumCost})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:offer", "account-resources"},
		Summary: fmt.Sprintf("Confirm offer %d at the server-quoted premium cost %d", request.OfferID, request.ConfirmedPremiumCost),
		Steps:   []Intent.Step{shopCommandStep("Confirm offer purchase", "oop", payload, 0)},
	}, nil
}

type shopOfferRequest struct {
	OfferID              int64   `json:"offerId"`
	Count                int64   `json:"count"`
	OptionIndexes        []int64 `json:"optionIndexes"`
	ConfirmedPremiumCost int64   `json:"confirmedPremiumCost,omitempty"`
}

func decodeShopOfferRequest(arguments json.RawMessage, requireConfirmation bool) (shopOfferRequest, error) {
	var request shopOfferRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return shopOfferRequest{}, err
	}
	if request.OfferID <= 0 || request.Count <= 0 {
		return shopOfferRequest{}, fmt.Errorf("offerId and count must be positive")
	}
	if len(request.OptionIndexes) == 0 {
		return shopOfferRequest{}, fmt.Errorf("optionIndexes must contain at least one selection")
	}
	for _, index := range request.OptionIndexes {
		if index < 0 {
			return shopOfferRequest{}, fmt.Errorf("optionIndexes cannot contain negative values")
		}
	}
	if requireConfirmation && request.ConfirmedPremiumCost < 0 {
		return shopOfferRequest{}, fmt.Errorf("confirmedPremiumCost cannot be negative")
	}
	return request, nil
}

func shopCommandStep(name string, opcode string, payload json.RawMessage, successCodes ...int) Intent.Step {
	step := commandStep(name, opcode, payload, opcode)
	step.SuccessCodes = successCodes
	step.CaptureResponse = true
	return step
}

func int64Value[T ~int64](value *T, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return int64(*value)
}
