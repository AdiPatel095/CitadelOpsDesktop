package Automation

import (
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
			Status: "waiting", Detail: "No construction-item targets are configured",
			NextCheckAt: snapshot.Now.Add(constructionCheckInterval),
		}, nil
	}
	if snapshot.GameData == nil {
		return Decision{}, fmt.Errorf("official game data is unavailable")
	}
	metadata, err := snapshot.GameData.ConstructionItemCatalog()
	if err != nil {
		return Decision{}, err
	}
	if snapshot.State.Inventory.ConstructionItemsObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionItemsObservedAt) >= constructionCheckInterval {
		return Decision{
			Status:              "ready",
			Detail:              "Refresh construction-item inventory",
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
				Status:              "ready",
				Detail:              fmt.Sprintf("Refresh construction-item slots at %s", castleName(castle)),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ReevaluateOnSuccess: true,
			}, nil
		}
		for _, target := range settings.Targets[castleKey] {
			representative, exists := metadata.DefinitionView(target.ID)
			if !exists || !representative.Temporary || representative.GroupID <= 0 || representative.VariantKey == "" {
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
								Status:              "ready",
								Detail:              fmt.Sprintf("Upgrade construction item %d to level %d at %s", equipped.item.ID, nextTier.Level, castleName(castle)),
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
				Status:              "ready",
				Detail:              fmt.Sprintf("Equip construction item %d at %s", tier.ID, castleName(castle)),
				NextCheckAt:         snapshot.Now.Add(30 * time.Second),
				Request:             &Intent.Request{Name: "construction.equip", Arguments: arguments},
				FollowUp:            &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
				ReevaluateOnSuccess: true,
			}, nil
		}
	}
	detail := "All configured construction-item targets are equipped"
	if targets == 0 {
		detail = "No valid official construction-item targets are configured"
	} else if occupiedHost > 0 {
		detail = fmt.Sprintf("%d construction-item target(s) are waiting for an occupied construction slot", occupiedHost)
	} else if outOfRange > 0 {
		detail = fmt.Sprintf("%d equipped construction-item target(s) are outside the configured level range", outOfRange)
	} else if missingInventory > 0 {
		detail = fmt.Sprintf("%d construction-item target(s) are waiting for matching inventory", missingInventory)
		if blockedShop > 0 {
			detail = fmt.Sprintf("%d construction-item target(s) have no matching live official shop offer", blockedShop)
		}
	} else if missingHost > 0 {
		detail = fmt.Sprintf("%d construction-item target(s) have no compatible observed building", missingHost)
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: nextCheck}, nil
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
			occupied, remaining := occupiedConstructionSlot(slot, item, castle.ConstructionSlotsObservedAt, now)
			if occupied {
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
			),
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
			Status:              "ready",
			Detail:              "Refresh live construction-item shop offers",
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
			Status:              "ready",
			Detail:              fmt.Sprintf("Buy construction item %d for configured targets", tier.ID),
			NextCheckAt:         snapshot.Now.Add(10 * time.Second),
			Request:             &Intent.Request{Name: "construction.purchase", Arguments: arguments},
			FollowUp:            &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
		}, "purchase"
	}
	return Decision{Status: "blocked", Detail: "No matching live or official trivial construction-item shop offer", NextCheckAt: snapshot.Now.Add(constructionCheckInterval)}, "no-offer"
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
	now time.Time,
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
	var nextAvailable time.Time
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
		availabilityKnown := true
		var buildingAvailableAt time.Time
		for _, slot := range castle.ConstructionSlots[instanceID] {
			item, known := metadata.DefinitionView(int64(slot.DefinitionID))
			if !known {
				occupied = true
				availabilityKnown = false
				continue
			}
			if item.Slot != targetSlot {
				continue
			}
			active, remaining := occupiedConstructionSlot(slot, item, castle.ConstructionSlotsObservedAt, now)
			if !active {
				continue
			}
			occupied = true
			if remaining == nil {
				availabilityKnown = false
				continue
			}
			candidate := now.Add(time.Duration(*remaining) * time.Second)
			if buildingAvailableAt.IsZero() || candidate.After(buildingAvailableAt) {
				buildingAvailableAt = candidate
			}
		}
		if !occupied {
			candidates = append(candidates, instanceID)
		} else if availabilityKnown && !buildingAvailableAt.IsZero() &&
			(nextAvailable.IsZero() || buildingAvailableAt.Before(nextAvailable)) {
			nextAvailable = buildingAvailableAt
		}
	}
	if len(candidates) == 0 {
		return 0, compatible, nextAvailable
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	return candidates[0], true, time.Time{}
}

func occupiedConstructionSlot(
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
