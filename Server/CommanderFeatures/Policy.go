package CommanderFeatures

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"CitadelDesktop/Server/State"
)

const Section = "automation.commanderFeatures"

const RequirementKindEquipmentEffect = "equipmentEffect"

type Configuration struct {
	Version      int                               `json:"version"`
	Assignments  map[string][]State.CommanderID    `json:"assignments"`
	Requirements map[string][]CommanderRequirement `json:"requirements,omitempty"`
}

type CommanderRequirement struct {
	Kind               string       `json:"kind"`
	EffectDefinitionID int64        `json:"effectDefinitionId,omitempty"`
	UnitID             State.UnitID `json:"unitId,omitempty"`
	MinimumValue       *float64     `json:"minimumValue,omitempty"`
	MaximumValue       *float64     `json:"maximumValue,omitempty"`
}

func Decode(raw json.RawMessage) (Configuration, error) {
	configuration := Configuration{}
	if err := json.Unmarshal(raw, &configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

func Candidates(
	gameState State.GameState,
	configuration Configuration,
	featureID string,
) ([]State.CommanderID, bool) {
	assigned, explicitlyAssigned := configuration.Assignments[featureID]
	requirements := configuration.Requirements[featureID]
	restricted := explicitlyAssigned || len(requirements) > 0
	if !restricted {
		return nil, false
	}

	candidateIDs := assigned
	if !explicitlyAssigned {
		candidateIDs = make([]State.CommanderID, 0, len(gameState.Commanders))
		for commanderID := range gameState.Commanders {
			candidateIDs = append(candidateIDs, commanderID)
		}
	}

	seen := map[State.CommanderID]struct{}{}
	candidates := make([]State.CommanderID, 0, len(candidateIDs))
	for _, commanderID := range candidateIDs {
		if commanderID < 0 {
			continue
		}
		if _, exists := gameState.Commanders[commanderID]; !exists {
			continue
		}
		if _, duplicate := seen[commanderID]; duplicate {
			continue
		}
		if !MeetsRequirements(gameState, commanderID, requirements) {
			continue
		}
		seen[commanderID] = struct{}{}
		candidates = append(candidates, commanderID)
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	return candidates, restricted
}

func AssignmentAllows(configuration Configuration, featureID string, commanderID State.CommanderID) bool {
	assigned, explicitlyAssigned := configuration.Assignments[featureID]
	if !explicitlyAssigned {
		return true
	}
	for _, candidateID := range assigned {
		if candidateID == commanderID {
			return true
		}
	}
	return false
}

func MeetsFeatureRequirements(
	gameState State.GameState,
	configuration Configuration,
	featureID string,
	commanderID State.CommanderID,
) bool {
	return MeetsRequirements(gameState, commanderID, configuration.Requirements[featureID])
}

func MeetsRequirements(
	gameState State.GameState,
	commanderID State.CommanderID,
	requirements []CommanderRequirement,
) bool {
	if len(requirements) == 0 {
		return true
	}
	commander, exists := gameState.Commanders[commanderID]
	if !exists {
		return false
	}
	for _, requirement := range requirements {
		switch strings.TrimSpace(requirement.Kind) {
		case RequirementKindEquipmentEffect:
			value, found := equipmentEffectValue(gameState, commander, requirement)
			if !found || requirement.MinimumValue != nil && value < *requirement.MinimumValue ||
				requirement.MaximumValue != nil && value > *requirement.MaximumValue {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func equipmentEffectValue(
	gameState State.GameState,
	commander State.CommanderState,
	requirement CommanderRequirement,
) (float64, bool) {
	if requirement.EffectDefinitionID <= 0 {
		return 0, false
	}
	total := 0.0
	found := false
	for _, equipmentID := range commander.Equipment {
		equipment, exists := gameState.Inventory.Equipment[equipmentID]
		if !exists {
			continue
		}
		for _, effect := range equipment.Effects {
			if effect.DefinitionID != requirement.EffectDefinitionID {
				continue
			}
			if requirement.UnitID > 0 {
				for index := 0; index+1 < len(effect.Values); index += 2 {
					if !sameInteger(effect.Values[index], int64(requirement.UnitID)) ||
						!validRequirementValue(effect.Values[index+1]) {
						continue
					}
					total += effect.Values[index+1]
					found = true
				}
				continue
			}
			if len(effect.Values) == 0 {
				continue
			}
			value := effect.Values[len(effect.Values)-1]
			if !validRequirementValue(value) {
				continue
			}
			total += value
			found = true
		}
	}
	return total, found
}

func sameInteger(value float64, expected int64) bool {
	return math.Abs(value-math.Round(value)) < 0.000001 && int64(math.Round(value)) == expected
}

func validRequirementValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
