package Models

import (
	"time"

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

func RiftMapCoordsPayload() map[string]interface{} {
	return gamestate.RiftMapCoordsPayload()
}

func MovementUpdatePayload() map[string]interface{} {
	return gamestate.MovementUpdatePayload()
}

func SetCastleFocusCoords(castleAID, kingdomID int) {
	gamestate.SetCastleFocusCoords(castleAID, kingdomID)
}

func GetCastleFocusCoords() (castleAID, kingdomID, mapPX, mapPY int) {
	return gamestate.GetCastleFocusCoords()
}

// PersistGameStateSnapshot writes the full GameState and map data to game_state_snapshot.json next to the executable.
func PersistGameStateSnapshot() {
	gamestate.PersistSnapshot()
}

// StartPeriodicGameStateSnapshots periodically persists game state so data survives disconnects and restarts.
func StartPeriodicGameStateSnapshots() {
	gamestate.StartPeriodicSnapshotSaver(90 * time.Second)
}

// ReadGameStateSnapshotMap reads the on-disk JSON snapshot (same file PersistGameStateSnapshot writes).
func ReadGameStateSnapshotMap() (map[string]interface{}, error) {
	return gamestate.ReadSnapshotForBroadcast()
}

const NumPlayerCastleSlots = castle.NumPlayerCastleSlots

func GetMapState() *mapstate.MapState { return mapstate.GetMapState() }

func GetSettingsState() *settings.SettingsState { return settings.GetSettingsState() }

// RandomAttackDelay returns a random pause from SettingsState min/max attack delays.
func RandomAttackDelay() time.Duration {
	return settings.RandomAttackDelay(settings.GetSettingsState())
}

// AttackDelayBetweenSends returns the random pause between consecutive attack dispatches.
// Uses Settings view delays when SchedulerSettings.json exists; otherwise 4–5s.
func AttackDelayBetweenSends() time.Duration { return settings.AttackDelayBetweenSends() }

type (
	VipState           = gamestate.VipState
	ActiveSubscription = gamestate.ActiveSubscription
	SubscriptionState  = gamestate.SubscriptionState

	PlayerCastleInfo        = castle.PlayerCastleInfo
	PlayerCastles           = castle.PlayerCastles
	CastleFocus             = castle.CastleFocus
	CastleTroops            = castle.CastleTroops
	BuildingData            = castle.BuildingData
	GCAConstructionBuilding = castle.GCAConstructionBuilding
	GCAConstructionSlot     = castle.GCAConstructionSlot
	CastleTroopData         = castle.CastleTroopData
	CastleResourcesAmount   = castle.CastleResourcesAmount
	CastleProductionTotal   = castle.CastleProductionTotal
	CastleStorageMax        = castle.CastleStorageMax

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
	AllianceMember       = alliance.AllianceMember
	BirdLocation         = alliance.BirdLocation
	PlayerCastleLocation = alliance.PlayerCastleLocation

	PlayerGlobalResources = resources.PlayerGlobalResources

	SettingsState       = settings.SettingsState
	SaveInCastleTroops  = settings.SaveInCastleTroops
	AutoBirdDelayConfig = settings.AutoBirdDelayConfig
	RecruitTroopsConfig = settings.RecruitTroopsConfig
	AutoToolConfig      = settings.AutoToolConfig
	AutoTCIConfig       = settings.AutoTCIConfig
	AutoBeriWorldConfig = settings.AutoBeriWorldConfig
	FeatureSchedule     = settings.FeatureSchedule
	FeatureScheduleSlot = settings.FeatureScheduleSlot
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

var SetCastleConstructionSlots = castle.SetCastleConstructionSlots

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
