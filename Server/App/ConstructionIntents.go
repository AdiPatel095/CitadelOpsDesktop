package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func planConstructionInventoryRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	return Intent.Plan{
		Claims: []string{"construction-inventory"}, Summary: "Refresh construction-item inventory", SummaryDescriptor: Localization.New("server.app.refresh_construction_item_inventory.5cd0c24e", "Refresh construction-item inventory", nil),
		Steps: []Intent.Step{
			constructionMenuStep(),
			commandStep("Refresh construction-item inventory", "gii", json.RawMessage(`{}`), "gii", Localization.New("server.app.refresh_construction_item_inventory.5cd0c24e", "Refresh construction-item inventory", nil)),
		},
	}, nil
}

func planConstructionPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID  State.CastleID  `json:"castleId"`
		ProductID State.PackageID `json:"productId"`
		Amount    int64           `json:"amount"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || request.CastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if input.GameData == nil || request.ProductID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("productId must reference the loaded official package catalog"), Localization.New("server.app.productid_must_reference_the.354a7676", "productId must reference the loaded official package catalog", nil))
	}
	catalog, err := input.GameData.Catalog("packages")
	if err != nil {
		return Intent.Plan{}, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(request.ProductID), 10))
	if !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("package %d is not in the current official catalog", request.ProductID), Localization.New("server.app.package_p_is_not.5209bee4", "package {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return Intent.Plan{}, err
	}
	packageType, _ := record.String("packageType")
	constructionItemID, _ := record.Int64("constructionItemID")
	if packageType != "constructionItem" || constructionItemID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("package %d is not a construction-item product", request.ProductID), Localization.New("server.app.package_p_is_not.b0f3cedd", "package {p0} is not a construction-item product", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID)}))
	}
	offers, offersObservedAt, offersFound := input.State.ConstructionOffersFor(castle.ID, castle.KingdomID)
	if !offersFound || offersObservedAt.IsZero() {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("construction-item shop offers have not been observed"), Localization.New("server.app.construction_item_shop_offers.d0847ee1", "construction-item shop offers have not been observed", nil))
	}
	liveAmount, offered := offers[request.ProductID]
	availableAmount := liveAmount
	if !offered || availableAmount <= 0 {
		if !GameData.ConstructionItemPackageIsTrivial(record) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("package %d is not in the current live construction-item offers", request.ProductID), Localization.New("server.app.package_p_is_not.924b6b03", "package {p0} is not in the current live construction-item offers", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductID)}))
		}
		availableAmount, _ = record.Int64("constructionItemAmount")
		if availableAmount <= 0 {
			availableAmount = 1
		}
	}
	if request.Amount <= 0 || request.Amount > availableAmount {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("amount must be between 1 and the available package amount %d", availableAmount), Localization.New("server.app.amount_must_be_between.1b2f37a5", "amount must be between 1 and the available package amount {p0}", Localization.Params{"p0": availableAmount}))
	}
	if input.State.Inventory.ConstructionItemsObservedAt.IsZero() {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("construction-item inventory has not been observed"), Localization.New("server.app.construction_item_inventory_has.3cc8607c", "construction-item inventory has not been observed", nil))
	}
	inventoryCount := State.ConstructionItemInventoryCount(input.State.Inventory.ConstructionItems)
	remainingCapacity := State.ConstructionItemInventorySpaceLeft(input.State.Inventory, time.Now().UTC())
	if remainingCapacity <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"construction-item inventory is full (%d/%d)",
			inventoryCount,
			State.ConstructionItemInventoryLimit,
		), Localization.New("server.app.construction_item_inventory_is.3fc29827", "construction-item inventory is full ({p0}/{p1})", Localization.Params{"p0": inventoryCount, "p1": State.ConstructionItemInventoryLimit}))
	}
	if request.Amount > remainingCapacity {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"purchase amount %d exceeds the construction-item inventory capacity remaining %d",
			request.Amount,
			remainingCapacity,
		), Localization.New("server.app.purchase_amount_p_exceeds.488a9695", "purchase amount {p0} exceeds the construction-item inventory capacity remaining {p1}", Localization.Params{"p0": request.Amount, "p1": remainingCapacity}))
	}
	payload, _ := json.Marshal(struct {
		ProductID State.PackageID `json:"PID"`
		BuildType int             `json:"BT"`
		TypeID    int             `json:"TID"`
		Amount    int64           `json:"AMT"`
		KingdomID State.KingdomID `json:"KID"`
		CastleID  State.CastleID  `json:"AID"`
		Premium   int             `json:"PC2"`
		BuildAux  int             `json:"BA"`
		Power     int             `json:"PWR"`
		Position  int             `json:"_PO"`
	}{request.ProductID, 0, 116, request.Amount, castle.KingdomID, castle.ID, -1, 0, 0, -1})
	steps := castleContextSteps(input, castle)
	steps = append(steps, constructionShopContextSteps(castle)...)
	// Mirror the official buy slider: ask the server for the remaining
	// inventory space right before buying, so the next guard evaluation runs
	// on the game's own number rather than a local estimate.
	steps = append(steps, constructionSpaceLeftStep())
	steps = append(steps, commandStep("Buy construction item", "sbp", payload, "sbp", Localization.New("server.app.buy_construction_item.ab605286", "Buy construction item", nil)))
	itemLabel := fmt.Sprintf("construction item %d", constructionItemID)
	itemName := itemLabel
	var itemLevel int64
	if itemCatalog, catalogErr := input.GameData.Catalog("constructionItems"); catalogErr == nil {
		if itemRaw, found := itemCatalog.Find(strconv.FormatInt(constructionItemID, 10)); found {
			if item, decodeErr := GameData.DecodeRecord(itemRaw); decodeErr == nil {
				if name, hasName := item.String("name"); hasName {
					if displayName := userFacingGameName(name); displayName != "" {
						itemLabel = displayName
					}
				}
				itemName = itemLabel
				if level, hasLevel := item.Int64("level"); hasLevel && level > 0 {
					itemLevel = level
					itemLabel += fmt.Sprintf(" (level %d)", level)
				}
			}
		}
	}
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"construction-inventory", "construction-shop", "account-resources",
		},
		Summary: fmt.Sprintf("Buy %d x %s from %s", request.Amount, itemLabel, castleLabel(castle)), SummaryDescriptor: constructionPurchaseDescriptor(input, constructionItemID, itemName, itemLevel, request.Amount, castleLabel(castle)),
		Steps: steps,
	}, nil
}
