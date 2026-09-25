package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const constructionCheckInterval = 15 * time.Minute

type ConstructionPolicy struct{}

type constructionSettings struct {
	Targets map[string][]constructionTarget `json:"targets"`
}

type constructionTarget struct {
	ID       int64 `json:"id"`
	Ceiling  int   `json:"amount"`
	MinLevel int   `json:"minLevel,omitempty"`
}

type constructionMetadata = GameData.ConstructionItemTier

type equippedConstruction struct {
	buildingID State.BuildingInstanceID
	slot       State.ConstructionSlot
	item       constructionMetadata
}

func NewConstructionPolicy() *ConstructionPolicy { return &ConstructionPolicy{} }

func (*ConstructionPolicy) ID() string { return "autoTCI" }

func (*ConstructionPolicy) EnabledKey() string { return "auto_tci" }

func (*ConstructionPolicy) WakeDomains() []string {
	return []string{"construction-items", "construction-offers", "inventory"}
}

func (*ConstructionPolicy) WakeSections() []string {
	return []string{"automation.constructionItems"}
}

func (*ConstructionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := constructionSettings{Targets: map[string][]constructionTarget{}}
	if !decodeSection(snapshot.Configuration, "automation.constructionItems", &settings) || len(settings.Targets) == 0 {
		return Decision{
			Status: "waiting", Detail: "No construction-item targets are configured", DetailDescriptor: Localization.New("server.automation.no_construction_item_targets.ab44deec", "No construction-item targets are configured", nil),
			NextCheckAt: snapshot.Now.Add(constructionCheckInterval),
		}, nil
	}
	if snapshot.GameData == nil {
		return Decision{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.automation.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	metadata, err := snapshot.GameData.ConstructionItemCatalog()
	if err != nil {
		return Decision{}, err
	}
	if snapshot.State.Inventory.ConstructionItemsObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionItemsObservedAt) >= constructionCheckInterval {
		return Decision{
			Status: "ready",
			Detail: "Refresh construction-item inventory", DetailDescriptor: Localization.New("server.automation.refresh_construction_item_inventory.5cd0c24e", "Refresh construction-item inventory", nil),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
		}, nil
	}
	missingInventory := 0
	missingHost := 0
	occupiedHost := 0
	outOfRange := 0
	blockedShop := 0
	targets := 0
	nextCheck := snapshot.Now.Add(constructionCheckInterval)
	for _, castleKey := range sortedNumericKeys(settings.Targets) {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists || len(settings.Targets[castleKey]) == 0 {
			continue
		}
		if castle.ConstructionSlotsObservedAt.IsZero() ||
			snapshot.Now.Sub(castle.ConstructionSlotsObservedAt) >= constructionCheckInterval {
			arguments, _ := json.Marshal(map[string]any{"castleId": castle.ID})
			return Decision{
				Status: "ready",
				Detail: fmt.Sprintf("Refresh construction-item slots at %s", castleName(castle)), DetailDescriptor: Localization.New("server.automation.refresh_construction_item_slots.e7899b20", "Refresh construction-item slots at {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ReevaluateOnSuccess: true,
			}, nil
		}
		for _, target := range settings.Targets[castleKey] {
			representative, exists := metadata.DefinitionView(target.ID)
			if !exists || !representative.Temporary || representative.GroupID <= 0 ||
				representative.VariantKey == "" || !representative.SlotKnown {
				continue
			}
			tiers := metadata.TiersView(representative.VariantKey)
			targets++
			floor := target.MinLevel
			if floor <= 0 {
				floor = 1
			}
			ceiling := target.Ceiling
			if ceiling < floor {
				ceiling = floor
			}
			equipped, hasEquipped := equippedConstructionForVariant(
				castle, metadata, representative.VariantKey, snapshot.Now,
			)
			if hasEquipped {
				if nextTier, hasNext := nextConstructionTier(tiers, State.ConstructionItemID(equipped.item.ID)); hasNext &&
					nextTier.Level <= ceiling && equipped.slot.RemainingSec != nil {
					remaining := *equipped.slot.RemainingSec
					if remaining <= 300 {
						offerCode, mapped := constructionUpgradeCode(nextTier.Level)
						if mapped {
							arguments, _ := json.Marshal(map[string]any{
								"castleId": castle.ID, "buildingInstanceId": equipped.buildingID,
								"constructionItemId": equipped.item.ID, "slot": equipped.slot.Slot, "offerCode": offerCode,
							})
							return Decision{
								Status: "ready",
								Detail: fmt.Sprintf("Upgrade construction item %d to level %d at %s", equipped.item.ID, nextTier.Level, castleName(castle)), DetailDescriptor: Localization.New("server.automation.upgrade_construction_item_p.f8050e4c", "Upgrade construction item {p0} to level {p1} at {p2}", Localization.Params{"p0": fmt.Sprintf("%d", equipped.item.ID), "p1": nextTier.Level, "p2": fmt.Sprintf("%s", castleName(castle))}),
								NextCheckAt:         snapshot.Now.Add(10 * time.Second),
								Request:             &Intent.Request{Name: "construction.upgrade", Arguments: arguments},
								ReevaluateOnSuccess: true,
							}, nil
						}
					}
					nextCheck = earlierConstructionCheck(nextCheck, snapshot.Now, remaining-300)
					continue
				}
				if equipped.item.Level >= floor && equipped.item.Level <= ceiling {
					if equipped.slot.RemainingSec != nil {
						remaining := *equipped.slot.RemainingSec
						nextCheck = earlierConstructionCheck(nextCheck, snapshot.Now, remaining)
						if remaining > 120 {
							if !constructionInventoryAvailable(tiers, snapshot.State.Inventory.ConstructionItems, floor, ceiling) {
								nextCheck = earlierConstructionCheck(nextCheck, snapshot.Now, remaining-120)
							}
						} else if !constructionInventoryAvailable(tiers, snapshot.State.Inventory.ConstructionItems, floor, ceiling) {
							missingInventory++
							if decision, status := constructionPurchaseDecision(snapshot, tiers, floor, ceiling); status != "" {
								if decision.Request != nil || status == "inventory-full" {
									return decision, nil
								}
								blockedShop++
							}
						}
					}
					continue
				}
				outOfRange++
				if equipped.slot.RemainingSec != nil {
					nextCheck = earlierConstructionCheck(nextCheck, snapshot.Now, *equipped.slot.RemainingSec)
				}
				continue
			}
			hostID, compatibleHost, hostAvailableAt := constructionHost(
				castle, snapshot.GameData, metadata, representative.GroupID, representative.Slot, snapshot.Now,
			)
			if hostID <= 0 {
				if compatibleHost {
					occupiedHost++
					if !hostAvailableAt.IsZero() && hostAvailableAt.Before(nextCheck) {
						nextCheck = hostAvailableAt
					}
				} else {
					missingHost++
				}
				continue
			}
			tier, available := bestConstructionInventoryTier(
				tiers, snapshot.State.Inventory.ConstructionItems, floor, ceiling,
			)
			if !available {
				missingInventory++
				if decision, status := constructionPurchaseDecision(snapshot, tiers, floor, ceiling); status != "" {
					if decision.Request != nil || status == "inventory-full" {
						return decision, nil
					}
					blockedShop++
				}
				continue
			}
			arguments, _ := json.Marshal(map[string]any{
				"castleId": castleID, "buildingInstanceId": hostID,
				"constructionItemId": tier.ID, "slot": representative.Slot, "mode": 0,
			})
			return Decision{
				Status: "ready",
				Detail: fmt.Sprintf("Equip construction item %d at %s", tier.ID, castleName(castle)), DetailDescriptor: Localization.New("server.automation.equip_construction_item_p.5f854063", "Equip construction item {p0} at {p1}", Localization.Params{"p0": fmt.Sprintf("%d", tier.ID), "p1": fmt.Sprintf("%s", castleName(castle))}),
				NextCheckAt:         snapshot.Now.Add(30 * time.Second),
				Request:             &Intent.Request{Name: "construction.equip", Arguments: arguments},
				FollowUp:            &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
				ReevaluateOnSuccess: true,
			}, nil
		}
	}
	detail := "All configured construction-item targets are equipped"
	var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.all_configured_construction_item.1f951d18", "All configured construction-item targets are equipped", nil)
	if targets == 0 {
		detail = "No valid official construction-item targets are configured"
		detailLocalizationMessage = Localization.New("server.automation.no_valid_official_construction.634f17c2", "No valid official construction-item targets are configured", nil)
	} else if occupiedHost > 0 {
		detail = fmt.Sprintf(
			"%d construction-item target(s) are waiting for an occupied construction slot; a refreshed slot snapshot must confirm removal before replacement",
			occupiedHost,
		)
		detailLocalizationMessage = Localization.New("server.automation.p_construction_item_target.5e66a184", "{p0, number} construction-item target(s) are waiting for an occupied construction slot; a refreshed slot snapshot must confirm removal before replacement", Localization.Params{"p0": occupiedHost})
	} else if outOfRange > 0 {
		detail = fmt.Sprintf("%d equipped construction-item target(s) are outside the configured level range", outOfRange)
		detailLocalizationMessage = Localization.New("server.automation.p_equipped_construction_item.9fac8579", "{p0, number} equipped construction-item target(s) are outside the configured level range", Localization.Params{"p0": outOfRange})
	} else if missingInventory > 0 {
		detail = fmt.Sprintf("%d construction-item target(s) are waiting for matching inventory", missingInventory)
		detailLocalizationMessage = Localization.New("server.automation.p_construction_item_target.7287a6c9", "{p0, number} construction-item target(s) are waiting for matching inventory", Localization.Params{"p0": missingInventory})
		if blockedShop > 0 {
			detail = fmt.Sprintf("%d construction-item target(s) have no matching live official shop offer", blockedShop)
			detailLocalizationMessage = Localization.New("server.automation.p_construction_item_target.84811bfe", "{p0, number} construction-item target(s) have no matching live official shop offer", Localization.Params{"p0": blockedShop})
		}
	} else if missingHost > 0 {
		detail = fmt.Sprintf("%d construction-item target(s) have no compatible observed building", missingHost)
		detailLocalizationMessage = Localization.New("server.automation.p_construction_item_target.b040dddf", "{p0, number} construction-item target(s) have no compatible observed building", Localization.Params{"p0": missingHost})
	}
	return Decision{Status: "idle", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), NextCheckAt: nextCheck}, nil
}

func equippedConstructionForVariant(
	castle State.CastleState,
	metadata *GameData.ConstructionItemCatalog,
	variantKey string,
	now time.Time,
) (equippedConstruction, bool) {
	buildingIDs := make([]State.BuildingInstanceID, 0, len(castle.ConstructionSlots))
	for buildingID := range castle.ConstructionSlots {
		buildingIDs = append(buildingIDs, buildingID)
	}
	sort.Slice(buildingIDs, func(left, right int) bool { return buildingIDs[left] < buildingIDs[right] })
	for _, buildingID := range buildingIDs {
		slots := append([]State.ConstructionSlot(nil), castle.ConstructionSlots[buildingID]...)
		sort.Slice(slots, func(left, right int) bool { return slots[left].Slot < slots[right].Slot })
		for _, slot := range slots {
			item, exists := metadata.DefinitionView(int64(slot.DefinitionID))
			if !exists || item.VariantKey != variantKey {
				continue
			}
			active, remaining := activeConstructionEffect(slot, item, castle.ConstructionSlotsObservedAt, now)
			if active {
				slot.RemainingSec = remaining
				return equippedConstruction{buildingID: buildingID, slot: slot, item: item}, true
			}
		}
	}
	return equippedConstruction{}, false
}

func nextConstructionTier(tiers []constructionMetadata, currentID State.ConstructionItemID) (constructionMetadata, bool) {
	currentLevel := 0
	for _, tier := range tiers {
		if tier.ID == int64(currentID) {
			currentLevel = tier.Level
			break
		}
	}
	if currentLevel <= 0 {
		return constructionMetadata{}, false
	}
	next := constructionMetadata{}
	for _, tier := range tiers {
		if tier.Level > currentLevel && (next.Level == 0 || tier.Level < next.Level) {
			next = tier
		}
	}
	return next, next.ID > 0
}

func constructionUpgradeCode(targetLevel int) (int, bool) {
	switch targetLevel {
	case 2:
		return 2000, true
	case 3:
		return 2001, true
	case 4:
		return 2002, true
	default:
		return 0, false
	}
}

func constructionInventoryAvailable(
	tiers []constructionMetadata,
	inventory map[State.ConstructionItemID]int64,
	floor int,
	ceiling int,
) bool {
	_, available := bestConstructionInventoryTier(tiers, inventory, floor, ceiling)
	return available
}

func constructionPurchaseDecision(
	snapshot Snapshot,
	tiers []constructionMetadata,
	floor int,
	ceiling int,
) (Decision, string) {
	inventoryCount := State.ConstructionItemInventoryCount(snapshot.State.Inventory.ConstructionItems)
	// The server's own space-left answer (csp) is the fullness oracle when
	// fresh; the softcap estimate is the fallback, exactly as the official
	// client does it. A stale local count must never block the blacksmith.
	remainingCapacity := State.ConstructionItemInventorySpaceLeft(snapshot.State.Inventory, snapshot.Now)
	if remainingCapacity <= 0 {
		return Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Construction-item inventory is full (%d/%d)",
				inventoryCount,
				State.ConstructionItemInventoryLimit,
			), DetailDescriptor: Localization.New("server.automation.construction_item_inventory_is.931b8391", "Construction-item inventory is full ({p0}/{p1})", Localization.Params{"p0": inventoryCount, "p1": State.ConstructionItemInventoryLimit}),
			NextCheckAt: snapshot.Now.Add(constructionCheckInterval),
		}, "inventory-full"
	}
	mainCastle, exists := constructionShopCastle(snapshot.State)
	if !exists {
		return Decision{}, "no-main-castle"
	}
	offers, offersObservedAt, offersFound := snapshot.State.ConstructionOffersFor(mainCastle.ID, mainCastle.KingdomID)
	if !offersFound || offersObservedAt.IsZero() ||
		snapshot.Now.Sub(offersObservedAt) >= constructionCheckInterval {
		arguments, _ := json.Marshal(map[string]any{"castleId": mainCastle.ID})
		return Decision{
			Status: "ready",
			Detail: "Refresh live construction-item shop offers", DetailDescriptor: Localization.New("server.automation.refresh_live_construction_item.c2231963", "Refresh live construction-item shop offers", nil),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "construction.shop", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, "refresh-shop"
	}
	ascending := append([]constructionMetadata(nil), tiers...)
	sort.Slice(ascending, func(left, right int) bool { return ascending[left].Level < ascending[right].Level })
	for _, tier := range ascending {
		if tier.Level < floor || tier.Level > ceiling {
			continue
		}
		products, err := snapshot.GameData.ConstructionShopProducts(tier.ID)
		if err != nil {
			continue
		}
		selected := GameData.ConstructionShopProduct{}
		amount := int64(0)
		for _, product := range products {
			liveAmount := offers[State.PackageID(product.PackageID)]
			if liveAmount <= 0 {
				continue
			}
			selected = product
			amount = min(product.Amount, liveAmount)
			break
		}
		if selected.PackageID <= 0 {
			for _, product := range products {
				if !product.Trivial {
					continue
				}
				selected = product
				amount = product.Amount
				break
			}
		}
		if selected.PackageID <= 0 {
			continue
		}
		if amount <= 0 {
			amount = 1
		}
		amount = min(amount, remainingCapacity)
		arguments, _ := json.Marshal(map[string]any{
			"castleId": mainCastle.ID, "productId": selected.PackageID, "amount": amount,
		})
		return Decision{
			Status: "ready",
			Detail: fmt.Sprintf("Buy construction item %d for configured targets", tier.ID), DetailDescriptor: Localization.New("server.automation.buy_construction_item_p.4a91a283", "Buy construction item {p0} for configured targets", Localization.Params{"p0": fmt.Sprintf("%d", tier.ID)}),
			NextCheckAt:         snapshot.Now.Add(10 * time.Second),
			Request:             &Intent.Request{Name: "construction.purchase", Arguments: arguments},
			FollowUp:            &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
		}, "purchase"
	}
	return Decision{Status: "blocked", Detail: "No matching live or official trivial construction-item shop offer", DetailDescriptor: Localization.New("server.automation.no_matching_live_or.c7c2e496", "No matching live or official trivial construction-item shop offer", nil), NextCheckAt: snapshot.Now.Add(constructionCheckInterval)}, "no-offer"
}

func constructionShopCastle(gameState State.GameState) (State.CastleState, bool) {
	castleIDs := sortedCastleIDs(gameState.Castles)
	for _, castleID := range castleIDs {
		castle := gameState.Castles[castleID]
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, true
		}
	}
	for _, castleID := range castleIDs {
		castle := gameState.Castles[castleID]
		if castle.KingdomID == 0 {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

func bestConstructionInventoryTier(
	tiers []constructionMetadata,
	inventory map[State.ConstructionItemID]int64,
	floor int,
	ceiling int,
) (constructionMetadata, bool) {
	best := constructionMetadata{}
	for _, tier := range tiers {
		if tier.Level < floor || tier.Level > ceiling || inventory[State.ConstructionItemID(tier.ID)] <= 0 {
			continue
		}
		if best.ID == 0 || tier.Level < best.Level || (tier.Level == best.Level && tier.ID < best.ID) {
			best = tier
		}
	}
	return best, best.ID > 0
}

func constructionHost(
	castle State.CastleState,
	store *GameData.Store,
	metadata *GameData.ConstructionItemCatalog,
	groupID int64,
	targetSlot int,
	_ time.Time,
) (State.BuildingInstanceID, bool, time.Time) {
	if store == nil {
		return 0, false, time.Time{}
	}
	buildings, err := store.BuildingCatalog()
	if err != nil {
		return 0, false, time.Time{}
	}
	candidates := make([]State.BuildingInstanceID, 0)
	compatible := false
	for instanceID, building := range castle.Buildings {
		definition, exists := buildings.DefinitionView(int64(building.DefinitionID))
		if !exists {
			continue
		}
		if !containsInt64(definition.ConstructionItemGroupIDs, groupID) {
			continue
		}
		compatible = true
		occupied := false
		for _, slot := range castle.ConstructionSlots[instanceID] {
			item, known := metadata.DefinitionView(int64(slot.DefinitionID))
			if !known || !item.SlotKnown {
				occupied = true
				continue
			}
			if item.Slot != targetSlot {
				continue
			}
			occupied = true
		}
		if !occupied {
			candidates = append(candidates, instanceID)
		}
	}
	if len(candidates) == 0 {
		return 0, compatible, time.Time{}
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	return candidates[0], true, time.Time{}
}

func activeConstructionEffect(
	slot State.ConstructionSlot,
	item constructionMetadata,
	observedAt time.Time,
	now time.Time,
) (bool, *int) {
	if !item.Temporary {
		return true, nil
	}
	if slot.RemainingSec == nil {
		return true, nil
	}
	remaining := *slot.RemainingSec
	if !observedAt.IsZero() && now.After(observedAt) {
		remaining -= int(now.Sub(observedAt) / time.Second)
	}
	return remaining > 0, &remaining
}

func earlierConstructionCheck(current time.Time, now time.Time, seconds int) time.Time {
	if seconds < 1 {
		seconds = 1
	}
	candidate := now.Add(time.Duration(seconds) * time.Second)
	if candidate.Before(current) {
		return candidate
	}
	return current
}
