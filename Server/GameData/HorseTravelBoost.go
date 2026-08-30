package GameData

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/State"
)

var (
	ErrHorseTravelBoostLayoutUnobserved = errors.New("horse travel boost layout is unobserved")
	ErrHorseTravelBoostUnavailable      = errors.New("horse travel boost is unavailable")
	ErrHorseTravelBoostConflict         = errors.New("horse travel boost buildings conflict")
)

type HorseTravelBoostTier int

const (
	HorseTravelBoostStandard HorseTravelBoostTier = 1
	HorseTravelBoostFast     HorseTravelBoostTier = 2
	HorseTravelBoostFastest  HorseTravelBoostTier = 3
)

type horseTravelBoostResolutionError struct {
	kind   error
	detail string
}

func (err horseTravelBoostResolutionError) Error() string { return err.detail }

func (err horseTravelBoostResolutionError) Is(target error) bool { return target == err.kind }

func HorseTravelBoostTierForSelection(value int) (HorseTravelBoostTier, bool) {
	switch value {
	case 1007:
		return HorseTravelBoostStandard, true
	case 1008:
		return HorseTravelBoostFast, true
	case 1009:
		return HorseTravelBoostFastest, true
	default:
		return 0, false
	}
}

type HorseTravelBoostDefinition struct {
	ID                   int64                `json:"id"`
	Tier                 HorseTravelBoostTier `json:"tier"`
	Name                 string               `json:"name,omitempty"`
	Family               string               `json:"family,omitempty"`
	UnitBoost            float64              `json:"unitBoost,omitempty"`
	MarketBoost          float64              `json:"marketBoost,omitempty"`
	SpyBoost             float64              `json:"spyBoost,omitempty"`
	CoinCostFactor       float64              `json:"coinCostFactor,omitempty"`
	PremiumCostFactor    float64              `json:"premiumCostFactor,omitempty"`
	BuildingDefinitionID int64                `json:"buildingDefinitionId"`
	BuildingName         string               `json:"buildingName,omitempty"`
	BuildingLevel        int                  `json:"buildingLevel,omitempty"`
}

type horseTravelBuildingDefinition struct {
	id        int64
	name      string
	level     int
	unlockIDs []int64
}

// ResolveHorseTravelBoost selects the official travel-booster ID unlocked by
// the source castle's Stable, Faction Stable, or Harbor. District-contained
// buildings and fixed-position Harbors are active even though the wire layout
// reports their grid coordinates as -1,-1 and therefore marks them unplaced.
func (store *Store) ResolveHorseTravelBoost(
	castle State.CastleState,
	tier HorseTravelBoostTier,
) (HorseTravelBoostDefinition, error) {
	if store == nil {
		return HorseTravelBoostDefinition{}, fmt.Errorf("official game data is unavailable")
	}
	if tier < HorseTravelBoostStandard || tier > HorseTravelBoostFastest {
		return HorseTravelBoostDefinition{}, fmt.Errorf("horse travel boost tier %d is invalid", tier)
	}
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return HorseTravelBoostDefinition{}, err
	}
	candidates, err := horseTravelBuildingDefinitions(castle, buildings)
	if err != nil {
		return HorseTravelBoostDefinition{}, err
	}
	if len(candidates) == 0 {
		if castle.Layout.ObservedAt.IsZero() {
			return HorseTravelBoostDefinition{}, horseTravelBoostResolutionError{
				kind:   ErrHorseTravelBoostLayoutUnobserved,
				detail: fmt.Sprintf("castle %d building layout has not been observed", castle.ID),
			}
		}
		return HorseTravelBoostDefinition{}, horseTravelBoostResolutionError{
			kind: ErrHorseTravelBoostUnavailable,
			detail: fmt.Sprintf(
				"castle %d has no Stable, Faction Stable, or Harbor with official horse unlocks in its layout",
				castle.ID,
			),
		}
	}
	if len(candidates) > 1 {
		labels := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			labels = append(labels, fmt.Sprintf("%s level %d (%d)", candidate.name, candidate.level, candidate.id))
		}
		sort.Strings(labels)
		return HorseTravelBoostDefinition{}, horseTravelBoostResolutionError{
			kind: ErrHorseTravelBoostConflict,
			detail: fmt.Sprintf(
				"castle %d has conflicting travel-booster buildings: %s",
				castle.ID, strings.Join(labels, ", "),
			),
		}
	}
	building := candidates[0]
	index := int(tier) - 1
	if index >= len(building.unlockIDs) {
		return HorseTravelBoostDefinition{}, horseTravelBoostResolutionError{
			kind: ErrHorseTravelBoostUnavailable,
			detail: fmt.Sprintf(
				"official %s level %d definition %d does not unlock horse tier %d",
				building.name, building.level, building.id, tier,
			),
		}
	}
	horseID := building.unlockIDs[index]
	horses, err := store.Catalog("horses")
	if err != nil {
		return HorseTravelBoostDefinition{}, err
	}
	raw, found := horses.Find(strconv.FormatInt(horseID, 10))
	if !found {
		return HorseTravelBoostDefinition{}, fmt.Errorf("official horse definition %d is unavailable", horseID)
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return HorseTravelBoostDefinition{}, fmt.Errorf("decode official horse definition %d: %w", horseID, err)
	}
	group, _ := record.String("group")
	if !strings.EqualFold(strings.TrimSpace(group), "Travelbooster") {
		return HorseTravelBoostDefinition{}, fmt.Errorf(
			"official horse definition %d is not a travel booster",
			horseID,
		)
	}
	name, _ := record.String("comment2")
	family, _ := record.String("comment1")
	unitBoost, _ := record.Float64("unitBoost")
	marketBoost, _ := record.Float64("marketBoost")
	spyBoost, _ := record.Float64("spyBoost")
	coinCostFactor, _ := record.Float64("costFactorC1")
	premiumCostFactor, _ := record.Float64("costFactorC2")
	return HorseTravelBoostDefinition{
		ID: horseID, Tier: tier, Name: name, Family: family,
		UnitBoost: unitBoost, MarketBoost: marketBoost, SpyBoost: spyBoost,
		CoinCostFactor: coinCostFactor, PremiumCostFactor: premiumCostFactor,
		BuildingDefinitionID: building.id, BuildingName: building.name, BuildingLevel: building.level,
	}, nil
}

func horseTravelBuildingDefinitions(
	castle State.CastleState,
	catalog *Catalog,
) ([]horseTravelBuildingDefinition, error) {
	byUnlockSet := map[string]horseTravelBuildingDefinition{}
	collections := []map[State.BuildingInstanceID]State.Building{
		castle.Buildings, castle.Layout.Objects, castle.Layout.Ground, castle.Layout.Fixed,
	}
	for _, collection := range collections {
		for _, building := range collection {
			// Ordinary stored buildings are absent from the castle layout maps.
			// Do not use Placed here: district children and fixed structures are
			// active layout entries whose wire coordinates are intentionally -1,-1.
			if building.DefinitionID <= 0 {
				continue
			}
			raw, found := catalog.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
			if !found {
				continue
			}
			record, err := DecodeRecord(raw)
			if err != nil {
				return nil, fmt.Errorf("decode official building definition %d: %w", building.DefinitionID, err)
			}
			unlockValue, found := record.String("unlockHorses")
			if !found || strings.TrimSpace(unlockValue) == "" {
				continue
			}
			unlockIDs, err := parseHorseUnlockIDs(unlockValue)
			if err != nil {
				return nil, fmt.Errorf(
					"decode official horse unlocks for building definition %d: %w",
					building.DefinitionID, err,
				)
			}
			name, _ := record.String("name")
			if strings.TrimSpace(name) == "" {
				name = "travel building"
			}
			level, _ := record.Int64("level")
			keyParts := make([]string, len(unlockIDs))
			for index, id := range unlockIDs {
				keyParts[index] = strconv.FormatInt(id, 10)
			}
			key := strings.Join(keyParts, ",")
			candidate := horseTravelBuildingDefinition{
				id: int64(building.DefinitionID), name: name, level: int(level), unlockIDs: unlockIDs,
			}
			if current, exists := byUnlockSet[key]; !exists || candidate.id < current.id {
				byUnlockSet[key] = candidate
			}
		}
	}
	result := make([]horseTravelBuildingDefinition, 0, len(byUnlockSet))
	for _, candidate := range byUnlockSet {
		result = append(result, candidate)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].id < result[right].id })
	return result, nil
}

func parseHorseUnlockIDs(value string) ([]int64, error) {
	fields := strings.Split(value, ",")
	result := make([]int64, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid horse definition %q", field)
		}
		result = append(result, id)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("horse unlock list is empty")
	}
	return result, nil
}
