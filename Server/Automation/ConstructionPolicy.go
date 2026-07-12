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
	missingInventory := 0
	missingHost := 0
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
			if equipped, remaining := equippedConstructionTarget(castle, metadata, representative.groupID, floor, ceiling); equipped {
				if remaining != nil && *remaining > 120 {
					candidate := snapshot.Now.Add(time.Duration(*remaining-120) * time.Second)
					if candidate.Before(nextCheck) {
						nextCheck = candidate
					}
				}
				continue
			}
			tier, available := bestConstructionInventoryTier(
				tiersByGroup[representative.groupID], snapshot.State.Inventory.ConstructionItems, floor, ceiling,
			)
			if !available {
				missingInventory++
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

func equippedConstructionTarget(
	castle State.CastleState,
	metadata map[State.ConstructionItemID]constructionMetadata,
	groupID int64,
	floor int,
	ceiling int,
) (bool, *int) {
	for _, slots := range castle.ConstructionSlots {
		for _, slot := range slots {
			item, exists := metadata[slot.DefinitionID]
			if exists && item.groupID == groupID && item.level >= floor && item.level <= ceiling {
				return true, slot.RemainingSec
			}
		}
	}
	return false, nil
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
