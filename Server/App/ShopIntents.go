package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func (application *Application) registerShopIntents() error {
	if err := application.registerAutoBoosterIntents(); err != nil {
		return err
	}
	if err := application.registerAutoBuyerIntents(); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name:        "shop.package.history",
			Description: "Request GBC purchase counters for stock-limited and rebuyable packages", DescriptionDescriptor: Localization.New("server.intent.description.82f88037", "Request GBC purchase counters for stock-limited and rebuyable packages", nil),
			Effect:           Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"castleId":12345678}`),
			Planner:          planShopPackageHistory,
		},
		{
			Name:        "shop.package.purchase",
			Description: "Send an explicit capture-shaped SBP purchase for an official package and shop table", DescriptionDescriptor: Localization.New("server.intent.description.483176cf", "Send an explicit capture-shaped SBP purchase for an official package and shop table", nil),
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"productId":3068,"tableId":116,"amount":10,"kingdomId":0,"castleId":-1,"buildType":0,"premiumCost":-1,"buyAll":0,"power":0,"position":-1}`),
			Planner:          planShopPackagePurchase,
		},
		{
			Name:        "shop.mercenary.refresh",
			Description: "Request the current Mercenary Post offers with the captured MPE shape", DescriptionDescriptor: Localization.New("server.intent.description.7a363ffc", "Request the current Mercenary Post offers with the captured MPE shape", nil),
			Effect:           Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{}`),
			Planner:          planShopMercenaryRefresh,
		},
		{
			Name:        "shop.mercenary.purchase",
			Description: "Attempt a Mercenary Post slot purchase with the observed MBS shape", DescriptionDescriptor: Localization.New("server.intent.description.7aa41cb4", "Attempt a Mercenary Post slot purchase with the observed MBS shape", nil),
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"slotId":1}`),
			Planner:          planShopMercenaryPurchase,
		},
		{
			Name:        "shop.offer.purchase",
			Description: "Send the first OOP purchase step; code 440 returns the server-confirmed premium price", DescriptionDescriptor: Localization.New("server.intent.description.2f92d1a0", "Send the first OOP purchase step; code 440 returns the server-confirmed premium price", nil),
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"offerId":5894,"count":1,"optionIndexes":[0]}`),
			Planner:          planShopOfferPurchase,
		},
		{
			Name:        "shop.offer.confirm",
			Description: "Confirm an OOP purchase using the exact CC2T value returned by code 440", DescriptionDescriptor: Localization.New("server.intent.description.3603641c", "Confirm an OOP purchase using the exact CC2T value returned by code 440", nil),
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	kingdomID := castle.KingdomID
	if request.KingdomID != nil {
		kingdomID = *request.KingdomID
		if kingdomID != castle.KingdomID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d belongs to kingdom %d, not %d", castle.ID, castle.KingdomID, kingdomID), Localization.New("server.app.castle_p_belongs_to.7e64d867", "castle {p0} belongs to kingdom {p1}, not {p2}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID), "p1": fmt.Sprintf("%d", castle.KingdomID), "p2": fmt.Sprintf("%d", kingdomID)}))
		}
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, kingdomID})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:purchase-history"},
		Summary: fmt.Sprintf("Load stock-limited package purchase counters for %s", castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.load_stock_limited_package.cfa7b8e7", "Load stock-limited package purchase counters for {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: []Intent.Step{shopCommandStep("Load package purchase history", "gbc", payload, 0)},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("productId must reference the loaded official package catalog"), Localization.New("server.app.productid_must_reference_the.354a7676", "productId must reference the loaded official package catalog", nil))
	}
	packages, err := input.GameData.Catalog("packages")
	if err != nil {
		return Intent.Plan{}, err
	}
	rawPackage, exists := packages.Find(strconv.FormatInt(int64(request.ProductID), 10))
	if !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("package %d is not in the current official catalog", request.ProductID), Localization.New("server.app.package_p_is_not.5209bee4", "package {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID)}))
	}
	packageRecord, err := GameData.DecodeRecord(rawPackage)
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("decode package %d: %w", request.ProductID, err), Localization.ErrorContext(Localization.New("server.app.decode_package_p.3f7263db", "decode package {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID)}), err))
	}
	if request.TableID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("tableId must be positive"), Localization.New("server.app.tableid_must_be_positive.76052e31", "tableId must be positive", nil))
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("amount must be positive"), Localization.New("server.app.amount_must_be_positive.86cf8f4f", "amount must be positive", nil))
	}
	kingdomID := int64Value(request.KingdomID, 0)
	castleID := int64Value(request.CastleID, -1)
	buildType := int64Value(request.BuildType, 0)
	premium := int64Value(request.Premium, -1)
	buyAll := int64Value(request.BuyAll, 0)
	power := int64Value(request.Power, 0)
	position := int64Value(request.Position, -1)
	if stock, limited := packageRecord.Int64("stock"); limited && stock > 0 {
		offers := input.State.Inventory.ConstructionOffers
		if castleID > 0 {
			scoped, _, found := input.State.ConstructionOffersFor(State.CastleID(castleID), State.KingdomID(kingdomID))
			if !found {
				return Intent.Plan{}, Localization.WithError(fmt.Errorf("package counters are unavailable for castle %d in kingdom %d", castleID, kingdomID), Localization.New("server.app.package_counters_are_unavailable.d78d6701", "package counters are unavailable for castle {p0} in kingdom {p1}", Localization.Params{"p0": fmt.Sprintf("%d", castleID), "p1": fmt.Sprintf("%d", kingdomID)}))
			}
			offers = scoped
		}
		purchased := offers[request.ProductID]
		if request.Amount > stock || purchased+request.Amount > stock {
			remaining := max(int64(0), stock-purchased)
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("amount exceeds package %d remaining stock %d", request.ProductID, remaining), Localization.New("server.app.amount_exceeds_package_p.a9043815", "amount exceeds package {p0} remaining stock {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID), "p1": remaining}))
		}
	}

	if kingdomID < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdomId cannot be negative"), Localization.New("server.app.kingdomid_cannot_be_negative.bb3f3cf0", "kingdomId cannot be negative", nil))
	}
	if castleID == 0 || castleID < -1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castleId must be -1 or a positive castle instance id"), Localization.New("server.app.castleid_must_be_or.b114f5b6", "castleId must be -1 or a positive castle instance id", nil))
	}
	if premium < -1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("premiumCost cannot be less than -1"), Localization.New("server.app.premiumcost_cannot_be_less.f58095b3", "premiumCost cannot be less than -1", nil))
	}
	if buyAll < 0 || power < 0 || position < -1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("buyAll and power cannot be negative, and position cannot be less than -1"), Localization.New("server.app.buyall_and_power_cannot.9374f419", "buyAll and power cannot be negative, and position cannot be less than -1", nil))
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
		Summary: fmt.Sprintf("Purchase official package %d from shop table %d", request.ProductID, request.TableID), SummaryDescriptor: Localization.New("server.app.purchase_official_package_p.dfe84fcb", "Purchase official package {p0} from shop table {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID), "p1": fmt.Sprintf("%d", request.TableID)}),
		Steps: []Intent.Step{shopCommandStep("Purchase shop package", "sbp", payload, 0)},
	}, nil
}

func planShopMercenaryRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims:  []string{"shop", "shop:mercenary"},
		Summary: "Load current Mercenary Post offers", SummaryDescriptor: Localization.New("server.app.load_current_mercenary_post.04ae0828", "Load current Mercenary Post offers", nil),
		Steps: []Intent.Step{shopCommandStep("Load Mercenary Post", "mpe", json.RawMessage(`{"MID":-1}`), 0)},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("slotId must be between 1 and 6"), Localization.New("server.app.slotid_must_be_between.5df61d22", "slotId must be between 1 and 6", nil))
	}
	payload, _ := json.Marshal(struct {
		SlotID int64 `json:"SID"`
	}{request.SlotID})
	return Intent.Plan{
		Claims:  []string{"shop", "shop:mercenary", "account-resources"},
		Summary: fmt.Sprintf("Purchase Mercenary Post slot %d", request.SlotID), SummaryDescriptor: Localization.New("server.app.purchase_mercenary_post_slot.b370bdd8", "Purchase Mercenary Post slot {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.SlotID)}),
		Steps: []Intent.Step{shopCommandStep("Purchase Mercenary Post slot", "mbs", payload, 0)},
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
		Summary: fmt.Sprintf("Submit offer %d purchase and capture its confirmation response", request.OfferID), SummaryDescriptor: Localization.New("server.app.submit_offer_p_purchase.fd588542", "Submit offer {p0} purchase and capture its confirmation response", Localization.Params{"p0": fmt.Sprintf("%d", request.OfferID)}),
		Steps: []Intent.Step{shopCommandStep("Submit offer purchase", "oop", payload, 0, 440)},
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
		Summary: fmt.Sprintf("Confirm offer %d at the server-quoted premium cost %d", request.OfferID, request.ConfirmedPremiumCost), SummaryDescriptor: Localization.New("server.app.confirm_offer_p_at.ddf92cd2", "Confirm offer {p0} at the server-quoted premium cost {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.OfferID), "p1": request.ConfirmedPremiumCost}),
		Steps: []Intent.Step{shopCommandStep("Confirm offer purchase", "oop", payload, 0)},
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
		return shopOfferRequest{}, Localization.WithError(fmt.Errorf("offerId and count must be positive"), Localization.New("server.app.offerid_and_count_must.9fed1e9c", "offerId and count must be positive", nil))
	}
	if len(request.OptionIndexes) == 0 {
		return shopOfferRequest{}, Localization.WithError(fmt.Errorf("optionIndexes must contain at least one selection"), Localization.New("server.app.optionindexes_must_contain_at.c7ea704d", "optionIndexes must contain at least one selection", nil))
	}
	for _, index := range request.OptionIndexes {
		if index < 0 {
			return shopOfferRequest{}, Localization.WithError(fmt.Errorf("optionIndexes cannot contain negative values"), Localization.New("server.app.optionindexes_cannot_contain_negative.1f258562", "optionIndexes cannot contain negative values", nil))
		}
	}
	if requireConfirmation && request.ConfirmedPremiumCost < 0 {
		return shopOfferRequest{}, Localization.WithError(fmt.Errorf("confirmedPremiumCost cannot be negative"), Localization.New("server.app.confirmedpremiumcost_cannot_be_negative.498bc54a", "confirmedPremiumCost cannot be negative", nil))
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
