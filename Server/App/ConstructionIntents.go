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

func planConstructionInventoryRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	return Intent.Plan{
		Claims: []string{"construction-inventory"}, Summary: "Refresh construction-item inventory",
		Steps: []Intent.Step{
			constructionMenuStep(),
			commandStep("Refresh construction-item inventory", "gii", json.RawMessage(`{}`), "gii"),
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
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if input.GameData == nil || request.ProductID <= 0 {
		return Intent.Plan{}, fmt.Errorf("productId must reference the loaded official package catalog")
	}
	catalog, err := input.GameData.Catalog("packages")
	if err != nil {
		return Intent.Plan{}, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(request.ProductID), 10))
	if !exists {
		return Intent.Plan{}, fmt.Errorf("package %d is not in the current official catalog", request.ProductID)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return Intent.Plan{}, err
	}
	packageType, _ := record.String("packageType")
	constructionItemID, _ := record.Int64("constructionItemID")
	if packageType != "constructionItem" || constructionItemID <= 0 {
		return Intent.Plan{}, fmt.Errorf("package %d is not a construction-item product", request.ProductID)
	}
	offers, offersObservedAt, offersFound := input.State.ConstructionOffersFor(castle.ID, castle.KingdomID)
	if !offersFound || offersObservedAt.IsZero() {
		return Intent.Plan{}, fmt.Errorf("construction-item shop offers have not been observed")
	}
	liveAmount, offered := offers[request.ProductID]
	availableAmount := liveAmount
	if !offered || availableAmount <= 0 {
		if !GameData.ConstructionItemPackageIsTrivial(record) {
			return Intent.Plan{}, fmt.Errorf("package %d is not in the current live construction-item offers", request.ProductID)
		}
		availableAmount, _ = record.Int64("constructionItemAmount")
		if availableAmount <= 0 {
			availableAmount = 1
		}
	}
	if request.Amount <= 0 || request.Amount > availableAmount {
		return Intent.Plan{}, fmt.Errorf("amount must be between 1 and the available package amount %d", availableAmount)
	}
	if input.State.Inventory.ConstructionItemsObservedAt.IsZero() {
		return Intent.Plan{}, fmt.Errorf("construction-item inventory has not been observed")
	}
	inventoryCount := State.ConstructionItemInventoryCount(input.State.Inventory.ConstructionItems)
	remainingCapacity := State.ConstructionItemInventoryLimit - inventoryCount
	if remainingCapacity <= 0 {
		return Intent.Plan{}, fmt.Errorf(
			"construction-item inventory is full (%d/%d)",
			inventoryCount,
			State.ConstructionItemInventoryLimit,
		)
	}
	if request.Amount > remainingCapacity {
		return Intent.Plan{}, fmt.Errorf(
			"purchase amount %d exceeds the construction-item inventory capacity remaining %d",
			request.Amount,
			remainingCapacity,
		)
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
	steps = append(steps, commandStep("Buy construction item", "sbp", payload, "sbp"))
	itemLabel := fmt.Sprintf("construction item %d", constructionItemID)
	if itemCatalog, catalogErr := input.GameData.Catalog("constructionItems"); catalogErr == nil {
		if itemRaw, found := itemCatalog.Find(strconv.FormatInt(constructionItemID, 10)); found {
			if item, decodeErr := GameData.DecodeRecord(itemRaw); decodeErr == nil {
				if name, hasName := item.String("name"); hasName {
					if displayName := userFacingGameName(name); displayName != "" {
						itemLabel = displayName
					}
				}
				if level, hasLevel := item.Int64("level"); hasLevel && level > 0 {
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
		Summary: fmt.Sprintf("Buy %d x %s from %s", request.Amount, itemLabel, castleLabel(castle)),
		Steps:   steps,
	}, nil
}
