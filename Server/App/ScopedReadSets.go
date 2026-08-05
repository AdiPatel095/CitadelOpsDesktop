package App

import (
	"encoding/json"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func buildingReadSet(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	castleID, err := readSetCastleID(arguments)
	if err != nil {
		return nil, err
	}
	keys := []State.PartitionKey{
		State.SessionPartition(input.State, State.CapabilitySessionContext),
		State.AccountPartition(input.State, State.CapabilityCastleDirectory),
		State.AccountPartition(input.State, State.CapabilityAccountWallet),
		State.AccountPartition(input.State, State.CapabilityEconomy),
		State.AccountPartition(input.State, State.CapabilityInventory),
	}
	if castleID > 0 {
		keys = append(keys,
			State.CastlePartition(input.State, State.CapabilityBuildings, castleID),
			State.CastlePartition(input.State, State.CapabilityBuildingQueue, castleID),
			State.CastlePartition(input.State, State.CapabilityConstructionItems, castleID),
			State.CastlePartition(input.State, State.CapabilityEconomy, castleID),
		)
	}
	return keys, nil
}

func constructionReadSet(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	castleID, err := readSetCastleID(arguments)
	if err != nil {
		return nil, err
	}
	keys := []State.PartitionKey{
		State.SessionPartition(input.State, State.CapabilitySessionContext),
		State.AccountPartition(input.State, State.CapabilityCastleDirectory),
		State.AccountPartition(input.State, State.CapabilityAccountWallet),
		State.AccountPartition(input.State, State.CapabilityEconomy),
		State.AccountPartition(input.State, State.CapabilityConstructionInventory),
	}
	if castleID > 0 {
		keys = append(keys,
			State.CastlePartition(input.State, State.CapabilityBuildings, castleID),
			State.CastlePartition(input.State, State.CapabilityBuildingQueue, castleID),
			State.CastlePartition(input.State, State.CapabilityConstructionItems, castleID),
			State.CastlePartition(input.State, State.CapabilityEconomy, castleID),
			State.CastlePartition(input.State, State.CapabilityConstructionCommerce, castleID),
		)
	}
	return keys, nil
}

func riftMaidenReadSet(
	input Intent.PlanningContext,
	_ json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	return []State.PartitionKey{
		State.SessionPartition(input.State, State.CapabilitySessionContext),
		State.AccountPartition(input.State, State.CapabilityCastleDirectory),
		State.AccountPartition(input.State, State.CapabilityBuildings),
		State.AccountPartition(input.State, State.CapabilityGarrison),
		State.AccountPartition(input.State, State.CapabilityLeaders),
		State.AccountPartition(input.State, State.CapabilityEquipment),
		State.AccountPartition(input.State, State.CapabilityWorldMap),
		State.AccountPartition(input.State, State.CapabilityEvents),
	}, nil
}

func riftReplayReadSet(
	input Intent.PlanningContext,
	_ json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	return []State.PartitionKey{
		State.SessionPartition(input.State, State.CapabilitySessionContext),
		State.AccountPartition(input.State, State.CapabilityCastleDirectory),
		State.AccountPartition(input.State, State.CapabilityBuildings),
		State.AccountPartition(input.State, State.CapabilityGarrison),
		State.AccountPartition(input.State, State.CapabilityLeaders),
		State.AccountPartition(input.State, State.CapabilityEvents),
		State.AccountPartition(input.State, State.CapabilityAutomation),
	}, nil
}

func riftTemplateReadSet(
	input Intent.PlanningContext,
	_ json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	return []State.PartitionKey{
		State.AccountPartition(input.State, State.CapabilityEvents),
	}, nil
}

func riftTemplateDeleteReadSet(
	input Intent.PlanningContext,
	_ json.RawMessage,
	_ Intent.Plan,
) ([]State.PartitionKey, error) {
	return []State.PartitionKey{
		State.AccountPartition(input.State, State.CapabilityEvents),
		State.AccountPartition(input.State, State.CapabilityAutomation),
	}, nil
}

func readSetCastleID(arguments json.RawMessage) (State.CastleID, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if len(arguments) == 0 {
		return 0, nil
	}
	if err := json.Unmarshal(arguments, &request); err != nil {
		return 0, err
	}
	return request.CastleID, nil
}
