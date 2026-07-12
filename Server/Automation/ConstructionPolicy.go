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

type constructionMetadata struct {
	id      State.ConstructionItemID
	groupID int64
	level   int
	slot    int
}

type equippedConstruction struct {
	buildingID State.BuildingInstanceID
	slot       State.ConstructionSlot
	item       constructionMetadata
}

func NewConstructionPolicy() *ConstructionPolicy { return &ConstructionPolicy{} }

func (*ConstructionPolicy) ID() string { return "autoTCI" }

func (*ConstructionPolicy) EnabledKey() string { return "auto_tci" }

func (*ConstructionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := constructionSettings{Targets: map[string][]constructionTarget{}}
	if !decodeSection(snapshot.Configuration, "automation.constructionItems", &settings) || len(settings.Targets) == 0 {
		return Decision{
			Status: "waiting", Detail: "No construction-item targets are configured",
			NextCheckAt: snapshot.Now.Add(constructionCheckInterval),
		}, nil
	}
	metadata, tiersByGroup, err := constructionCatalog(snapshot.GameData)
	if err != nil {
		return Decision{}, err
	}
	if snapshot.State.Inventory.ConstructionItemsObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionItemsObservedAt) >= constructionCheckInterval {
		return Decision{
			Status: "ready", Detail: "Refresh construction-item inventory",
			NextCheckAt: snapshot.Now.Add(2 * time.Second),
			Request:     &Intent.Request{Name: "construction.inventory.refresh", Arguments: json.RawMessage(`{}`)},
		}, nil
	}
	missingInventory := 0
	missingHost := 0
	blockedShop := 0
	targets := 0
	nextCheck := snapshot.Now.Add(constructionCheckInterval)
	for _, castleKey := range sortedNumericKeys(settings.Targets) {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists {
			continue
		}
		for _, target := range settings.Targets[castleKey] {
			representative, exists := metadata[State.ConstructionItemID(target.ID)]
			if !exists || representative.groupID <= 0 {
				continue
			}
			targets++
			floor := target.MinLevel
			if floor <= 0 {
				floor = 1
			}
			ceiling := target.Ceiling
			if ceiling < floor {
				ceiling = floor
			}
			equipped, hasEquipped := equippedConstructionForGroup(castle, metadata, representative.groupID)
			if hasEquipped {
				if nextTier, hasNext := nextConstructionTier(tiersByGroup[representative.groupID], equipped.item.id); hasNext &&
					nextTier.level >= floor && nextTier.level <= ceiling && equipped.slot.RemainingSec != nil {
					remaining := *equipped.slot.RemainingSec
					if remaining == 0 || remaining > 0 && remaining <= 300 {
						offerCode, mapped := constructionUpgradeCode(nextTier.level)
						if mapped {
							arguments, _ := json.Marshal(map[string]any{
								"castleId": castle.ID, "buildingInstanceId": equipped.buildingID,
								"slot": equipped.slot.Slot, "offerCode": offerCode,
							})
							return Decision{
								Status: "ready", Detail: fmt.Sprintf("Upgrade construction item %d to level %d at %s", equipped.item.id, nextTier.level, castleName(castle)),
								NextCheckAt: snapshot.Now.Add(10 * time.Second),
								Request:     &Intent.Request{Name: "construction.upgrade", Arguments: arguments},
							}, nil
						}
					}
					candidate := snapshot.Now.Add(time.Duration(max(0, remaining-300)) * time.Second)
					if candidate.Before(nextCheck) {
						nextCheck = candidate
					}
					continue
				}
				if equipped.item.level >= floor && equipped.item.level <= ceiling {
					if equipped.slot.RemainingSec != nil {
						remaining := *equipped.slot.RemainingSec
						if remaining > 120 {
							candidate := snapshot.Now.Add(time.Duration(remaining-120) * time.Second)
							if candidate.Before(nextCheck) {
								nextCheck = candidate
							}
						} else if !constructionInventoryAvailable(tiersByGroup[representative.groupID], snapshot.State.Inventory.ConstructionItems, floor, ceiling) {
							if decision, status := constructionPurchaseDecision(snapshot, tiersByGroup[representative.groupID], floor, ceiling); status != "" {
								if decision.Request != nil {
									return decision, nil
								}
								blockedShop++
							}
						}
					}
					continue
				}
			}
			tier, available := bestConstructionInventoryTier(
				tiersByGroup[representative.groupID], snapshot.State.Inventory.ConstructionItems, floor, ceiling,
			)
			if !available {
				missingInventory++
				if decision, status := constructionPurchaseDecision(snapshot, tiersByGroup[representative.groupID], floor, ceiling); status != "" {
					if decision.Request != nil {
						return decision, nil
					}
					blockedShop++
				}
				continue
			}
			hostID := constructionHost(castle, snapshot.GameData, metadata, representative.groupID)
			if hostID <= 0 {
				missingHost++
				continue
			}
			arguments, _ := json.Marshal(map[string]any{
				"castleId": castleID, "buildingInstanceId": hostID,
				"constructionItemId": tier.id, "slot": 0, "mode": 0,
			})
			return Decision{
				Status:      "ready",
				Detail:      fmt.Sprintf("Equip construction item %d at %s", tier.id, castleName(castle)),
				NextCheckAt: snapshot.Now.Add(30 * time.Second),
				Request:     &Intent.Request{Name: "construction.equip", Arguments: arguments},
			}, nil
		}
	}
	detail := "All configured construction-item targets are equipped"
	if targets == 0 {
		detail = "No valid official construction-item targets are configured"
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

func constructionCatalog(store *GameData.Store) (
	map[State.ConstructionItemID]constructionMetadata,
	map[int64][]constructionMetadata,
	error,
) {
	if store == nil {
		return nil, nil, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := store.Catalog("constructionItems")
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[State.ConstructionItemID]constructionMetadata, len(catalog.Rows()))
	byGroup := map[int64][]constructionMetadata{}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, _ := record.Int64("constructionItemID")
		groupID, _ := record.Int64("constructionItemGroupID")
		level, _ := record.Int64("level")
		slot, _ := record.Int64("slotTypeID")
		if id <= 0 || groupID <= 0 {
			continue
		}
		metadata := constructionMetadata{
			id: State.ConstructionItemID(id), groupID: groupID, level: int(level), slot: int(slot),
		}
		byID[metadata.id] = metadata
		byGroup[groupID] = append(byGroup[groupID], metadata)
	}
	for groupID := range byGroup {
		sort.Slice(byGroup[groupID], func(left, right int) bool {
			return byGroup[groupID][left].level > byGroup[groupID][right].level
		})
	}
	return byID, byGroup, nil
}

func equippedConstructionForGroup(
	castle State.CastleState,
	metadata map[State.ConstructionItemID]constructionMetadata,
	groupID int64,
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
			item, exists := metadata[slot.DefinitionID]
			if exists && item.groupID == groupID {
				return equippedConstruction{buildingID: buildingID, slot: slot, item: item}, true
			}
		}
	}
	return equippedConstruction{}, false
}

func nextConstructionTier(tiers []constructionMetadata, currentID State.ConstructionItemID) (constructionMetadata, bool) {
	currentLevel := 0
	for _, tier := range tiers {
		if tier.id == currentID {
			currentLevel = tier.level
			break
		}
	}
	if currentLevel <= 0 {
		return constructionMetadata{}, false
	}
	next := constructionMetadata{}
	for _, tier := range tiers {
		if tier.level > currentLevel && (next.level == 0 || tier.level < next.level) {
			next = tier
		}
	}
	return next, next.id > 0
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
	mainCastle, exists := constructionShopCastle(snapshot.State)
	if !exists {
		return Decision{}, "no-main-castle"
	}
	if snapshot.State.Inventory.ConstructionOffersObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionOffersObservedAt) >= constructionCheckInterval {
		arguments, _ := json.Marshal(map[string]any{"castleId": mainCastle.ID})
		return Decision{
			Status: "ready", Detail: "Refresh live construction-item shop offers",
			NextCheckAt: snapshot.Now.Add(2 * time.Second),
			Request:     &Intent.Request{Name: "construction.shop", Arguments: arguments},
		}, "refresh-shop"
	}
	ascending := append([]constructionMetadata(nil), tiers...)
	sort.Slice(ascending, func(left, right int) bool { return ascending[left].level < ascending[right].level })
	for _, tier := range ascending {
		if tier.level < floor || tier.level > ceiling {
			continue
		}
		products, err := snapshot.GameData.ConstructionShopProducts(int64(tier.id))
		if err != nil {
			continue
		}
		for _, product := range products {
			liveAmount := snapshot.State.Inventory.ConstructionOffers[State.PackageID(product.PackageID)]
			if liveAmount <= 0 {
				continue
			}
			amount := min(product.Amount, liveAmount)
			if amount <= 0 {
				amount = 1
			}
			arguments, _ := json.Marshal(map[string]any{
				"castleId": mainCastle.ID, "productId": product.PackageID, "amount": amount,
			})
			return Decision{
				Status: "ready", Detail: fmt.Sprintf("Buy construction item %d for configured targets", tier.id),
				NextCheckAt: snapshot.Now.Add(10 * time.Second),
				Request:     &Intent.Request{Name: "construction.purchase", Arguments: arguments},
			}, "purchase"
		}
	}
	return Decision{Status: "blocked", Detail: "No matching live official construction-item shop offer", NextCheckAt: snapshot.Now.Add(constructionCheckInterval)}, "no-offer"
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
	for _, tier := range tiers {
		if tier.level >= floor && tier.level <= ceiling && inventory[tier.id] > 0 {
			return tier, true
		}
	}
	return constructionMetadata{}, false
}

func constructionHost(
	castle State.CastleState,
	store *GameData.Store,
	metadata map[State.ConstructionItemID]constructionMetadata,
	groupID int64,
) State.BuildingInstanceID {
	for buildingID, slots := range castle.ConstructionSlots {
		for _, slot := range slots {
			if item, exists := metadata[slot.DefinitionID]; exists && item.groupID == groupID {
				return buildingID
			}
		}
	}
	if store == nil {
		return 0
	}
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return 0
	}
	candidates := make([]State.BuildingInstanceID, 0)
	for instanceID, building := range castle.Buildings {
		raw, exists := buildings.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
		if !exists {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		groups, _ := record.String("constructionItemGroupIDs")
		if _, allowed := commaSeparatedIDs(groups)[groupID]; !allowed {
			continue
		}
		if len(castle.ConstructionSlots[instanceID]) == 0 {
			candidates = append(candidates, instanceID)
		}
	}
	if len(candidates) == 0 {
		return 0
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	return candidates[0]
}
