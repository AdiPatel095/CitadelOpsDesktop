package Models

import (
	"CitadelDesktop/Server/Models/Alliance"
	"CitadelDesktop/Server/Models/Castle"
	"CitadelDesktop/Server/Models/Decoration"
	"CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/Models/GameState"
	"CitadelDesktop/Server/Models/MapState"
	"CitadelDesktop/Server/Models/Movement"
	"CitadelDesktop/Server/Models/Resources"
	"CitadelDesktop/Server/Models/SentBird"
	"CitadelDesktop/Server/Models/Settings"
)

// GameState and accessors — canonical definitions live in gamestate.
type GameState = gamestate.GameState

func GetGameState() *GameState { return gamestate.GetGameState() }

func CastleFocusMessagePayload() map[string]interface{} {
	return gamestate.CastleFocusMessagePayload()
}

func SetCastleFocusCoords(castleAID, kingdomID int) {
	gamestate.SetCastleFocusCoords(castleAID, kingdomID)
}

func GetCastleFocusCoords() (castleAID, kingdomID, mapPX, mapPY int) {
	return gamestate.GetCastleFocusCoords()
}

const NumPlayerCastleSlots = castle.NumPlayerCastleSlots

func GetMapState() *mapstate.MapState { return mapstate.GetMapState() }

func GetSettingsState() *settings.SettingsState { return settings.GetSettingsState() }

type (
	PlayerCastleInfo      = castle.PlayerCastleInfo
	PlayerCastles         = castle.PlayerCastles
	CastleFocus           = castle.CastleFocus
	CastleTroops          = castle.CastleTroops
	BuildingData          = castle.BuildingData
	CastleTroopData       = castle.CastleTroopData
	CastleResourcesAmount = castle.CastleResourcesAmount
	CastleProductionTotal = castle.CastleProductionTotal
	CastleStorageMax      = castle.CastleStorageMax

	EquipmentModel       = equipment.EquipmentModel
	Gem                  = equipment.Gem
	GemSlot              = equipment.GemSlot
	Stat                 = equipment.Stat
	CommStatModel        = equipment.CommStatModel
	CastStatModel        = equipment.CastStatModel
	CommActualModel      = equipment.CommActualModel
	CastActualModel      = equipment.CastActualModel
	PlayerEquipment      = equipment.PlayerEquipment
	EffectCoverageReport = equipment.EffectCoverageReport

	BirdMovement   = movement.BirdMovement
	GAMMovement    = movement.GAMMovement
	PlayerMovement = movement.PlayerMovement

	Alliance             = alliance.Alliance
	BirdLocation         = alliance.BirdLocation
	PlayerCastleLocation = alliance.PlayerCastleLocation

	PlayerGlobalResources = resources.PlayerGlobalResources

	SettingsState       = settings.SettingsState
	SaveInCastleTroops  = settings.SaveInCastleTroops
	AutoBirdDelayConfig = settings.AutoBirdDelayConfig
	RecruitTroopsConfig = settings.RecruitTroopsConfig
	TabPriority         = settings.TabPriority

	LoggedBird = sentbird.LoggedBird

	MapState  = mapstate.MapState
	MapNode   = mapstate.MapNode
	MapPlayer = mapstate.MapPlayer
)

const (
	Priority1 = settings.Priority1
	Priority2 = settings.Priority2
	Priority3 = settings.Priority3
	Ignored   = settings.Ignored
)

var (
	TroopIDs      = castle.TroopIDs
	ToolIDs       = castle.ToolIDs
	BuildingIDMap = castle.BuildingIDMap
)

// Stat application uses the same maps/functions as equipment (do not copy ceiling structs here).
var (
	ApplyCommCeiling   = equipment.ApplyCommCeiling
	ApplyCastCeiling   = equipment.ApplyCastCeiling
	CommStatUpdaterMap = equipment.CommStatUpdaterMap
	CastStatUpdaterMap = equipment.CastStatUpdaterMap
)

var SetCastleBuildingRows = castle.SetCastleBuildingRows

var (
	SentBirdLoad         = sentbird.Load
	SentBirdAppend       = sentbird.Append
	SentBirdReplaceBirds = sentbird.ReplaceBirds
)

var (
	IsTroop                          = castle.IsTroop
	IsTool                           = castle.IsTool
	GetBuildingInfo                  = castle.GetBuildingInfo
	IsDecorationPickupCandidateWID   = decoration.IsDecorationPickupCandidateWID
	DecorationSOBBlockedWID          = decoration.DecorationSOBBlockedWID
	IsEssentialCastleStructureByName = decoration.IsEssentialCastleStructureByName
	IsKnownDecorationWID             = decoration.IsKnownDecorationWID
	DecorationDisplayName            = decoration.DecorationDisplayName
	DecorationCatalogVersion         = decoration.DecorationCatalogVersion
)
