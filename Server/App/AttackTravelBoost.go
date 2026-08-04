package App

import (
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const defaultHorseTravelBoostID = -1

func validateHorseTravelBoostID(value int) error {
	switch value {
	case 0, -1, 1007, 1008, 1009:
		return nil
	default:
		return fmt.Errorf("horseTravelBoostId must be -1, 1007, 1008, or 1009")
	}
}

func horseTravelBoostFields(value int) (int, int) {
	switch value {
	case 1007, 1008, 1009:
		return value, 0
	default:
		return defaultHorseTravelBoostID, 1
	}
}

func resolveCastleHorseTravelBoostID(
	gameData *GameData.Store,
	castle State.CastleState,
	value int,
) (int, error) {
	if err := validateHorseTravelBoostID(value); err != nil {
		return 0, err
	}
	tier, selected := GameData.HorseTravelBoostTierForSelection(value)
	if !selected {
		return defaultHorseTravelBoostID, nil
	}
	definition, err := gameData.ResolveHorseTravelBoost(castle, tier)
	if err != nil {
		return 0, err
	}
	return int(definition.ID), nil
}

func resolveCastleHorseTravelBoostFields(
	gameData *GameData.Store,
	castle State.CastleState,
	value int,
) (int, int, error) {
	booster, err := resolveCastleHorseTravelBoostID(gameData, castle, value)
	if err != nil {
		return 0, 0, err
	}
	if booster == defaultHorseTravelBoostID {
		return booster, 1, nil
	}
	return booster, 0, nil
}

func applyCastleHorseTravelBoost(
	body *attackBody,
	gameData *GameData.Store,
	castle State.CastleState,
	value int,
) error {
	if body == nil {
		return fmt.Errorf("attack body is unavailable")
	}
	booster, premiumTravel, err := resolveCastleHorseTravelBoostFields(gameData, castle, value)
	if err != nil {
		return err
	}
	body.Booster = booster
	body.PremiumTravel = premiumTravel
	return nil
}
