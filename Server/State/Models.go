package State

import (
	"encoding/json"
	"strconv"
	"time"
)

const SchemaVersion = 2

type (
	PlayerID            int64
	AllianceID          int64
	CastleID            int64
	KingdomID           int64
	BuildingInstanceID  int64
	BuildingID          int64
	ConstructionItemID  int64
	UnitID              int64
	ResourceID          int64
	CurrencyID          int64
	CommanderID         int64
	CastellanID         int64
	MovementID          int64
	EquipmentInstanceID int64
	EquipmentID         int64
	GemInstanceID       int64
	GemID               int64
	PackageID           int64
)

type DefinitionRef struct {
	Collection string `json:"collection"`
	ID         int64  `json:"id"`
}

type SessionState struct {
	Mode                 string     `json:"mode"`
	Generation           uint64     `json:"generation"`
	BaselineGeneration   uint64     `json:"baselineGeneration"`
	ConnectionGeneration uint64     `json:"connectionGeneration"`
	Status               string     `json:"status"`
	LoggedIn             bool       `json:"loggedIn"`
	SocketReady          bool       `json:"socketReady"`
	BrowserID            string     `json:"browserId,omitempty"`
	BrowserName          string     `json:"browserName,omitempty"`
	ServerURL            string     `json:"serverUrl,omitempty"`
	Namespace            string     `json:"namespace,omitempty"`
	Detail               string     `json:"detail,omitempty"`
	CooldownUntil        *time.Time `json:"cooldownUntil,omitempty"`
	RetryAt              *time.Time `json:"retryAt,omitempty"`
	// LoginFailure describes the most recent failed game login while the
	// session is in a cooldown, error, or reconnecting state. It is cleared by
	// the next status change and never contains credential material.
	LoginFailure *LoginFailure `json:"loginFailure,omitempty"`
	ChangedAt    time.Time     `json:"changedAt"`
}

// LoginFailureClass is the sanitized classification of a failed game login.
// Orchestration decides on it: a cooldown is waited out, a suspended account or
// rejected credential needs the user, and an unknown fatal code is a reason to
// re-place the account with its stored credential rather than to keep retrying
// on a possibly corrupted runtime.
type LoginFailureClass string

const (
	// LoginFailureCooldown is the game's temporary lockout
	// (LOGIN_COOLDOWN_ACTIVE, code 453, payload "CD" = remaining seconds);
	// it clears on its own.
	LoginFailureCooldown LoginFailureClass = "cooldown"
	// LoginFailureClientVersionRejected means the announced client build was
	// refused; the runtime refreshes the official build and retries.
	LoginFailureClientVersionRejected LoginFailureClass = "client_version_rejected"
	// LoginFailureSuspended is IS_BANNED (code 27): the game refused the
	// account itself, temporarily (payload "RS" = remaining seconds) or
	// permanently. Only the user and Goodgame support can resolve it.
	LoginFailureSuspended LoginFailureClass = "suspended"
	// LoginFailureAccountDeleted is IS_BANNED with the "GDPR" flag: the
	// account was deactivated and is being deleted.
	LoginFailureAccountDeleted LoginFailureClass = "account_deleted"
	// LoginFailureInvalidCredentials covers INVALID_PASSWORD (20) and the
	// login-token rejections (409, 423): the credential itself was refused.
	LoginFailureInvalidCredentials LoginFailureClass = "invalid_credentials"
	// LoginFailureWrongServer covers PLAYER_NOT_FOUND (21), NO_AVATAR_CREATED
	// (26), and EXISTING_MAPPING_WRONG_SERVER (368): the account is not
	// playable on the selected server, so the server selection (or username)
	// must change.
	LoginFailureWrongServer LoginFailureClass = "wrong_server"
	// LoginFailureUnknown covers every code without an established meaning.
	LoginFailureUnknown LoginFailureClass = "unknown"
)

// LoginFailure is the structured outcome of one failed Background login. Code
// is the raw game response code so orchestration can classify codes this
// build does not know yet.
type LoginFailure struct {
	Code       int               `json:"code"`
	Class      LoginFailureClass `json:"class"`
	Fatal      bool              `json:"fatal"`
	ObservedAt time.Time         `json:"observedAt"`
	// SuspendedUntil is set for a temporary suspension from the game's
	// remaining-seconds payload; nil for permanent or unrelated failures.
	SuspendedUntil *time.Time `json:"suspendedUntil,omitempty"`
}

// knownLoginFailureCodes maps game login (lli) response codes to their class.
// The codes and names come from the official HTML5 client's
// BasicErrorConstants and the cases its LLI command handles.
var knownLoginFailureCodes = map[int]LoginFailureClass{
	20:  LoginFailureInvalidCredentials, // INVALID_PASSWORD
	21:  LoginFailureWrongServer,        // PLAYER_NOT_FOUND
	26:  LoginFailureWrongServer,        // NO_AVATAR_CREATED
	27:  LoginFailureSuspended,          // IS_BANNED
	368: LoginFailureWrongServer,        // EXISTING_MAPPING_WRONG_SERVER
	369: LoginFailureUnknown,            // UNEXPECTED_FACEBOOK_ERROR
	409: LoginFailureInvalidCredentials, // INVALID_LOGIN_TOKEN
	423: LoginFailureInvalidCredentials, // INVALID_GLOBALSERVER_LOGIN_TOKEN
	453: LoginFailureCooldown,           // LOGIN_COOLDOWN_ACTIVE
}

// LoginFailureCodeName returns the official client's constant name for a login
// response code, or an empty string for a code it does not name.
func LoginFailureCodeName(code int) string {
	switch code {
	case 0:
		return "ALL_OK"
	case 20:
		return "INVALID_PASSWORD"
	case 21:
		return "PLAYER_NOT_FOUND"
	case 26:
		return "NO_AVATAR_CREATED"
	case 27:
		return "IS_BANNED"
	case 368:
		return "EXISTING_MAPPING_WRONG_SERVER"
	case 369:
		return "UNEXPECTED_FACEBOOK_ERROR"
	case 409:
		return "INVALID_LOGIN_TOKEN"
	case 423:
		return "INVALID_GLOBALSERVER_LOGIN_TOKEN"
	case 453:
		return "LOGIN_COOLDOWN_ACTIVE"
	default:
		return ""
	}
}

// ClassifyLoginFailure returns the class for a login response code and whether
// the failure is fatal for the current bootstrap (retrying without a change is
// pointless). Cooldowns and client-version rejections are the only non-fatal
// classes.
func ClassifyLoginFailure(code int, clientVersionRejected bool) (LoginFailureClass, bool) {
	if clientVersionRejected {
		return LoginFailureClientVersionRejected, false
	}
	if class, known := knownLoginFailureCodes[code]; known {
		return class, class != LoginFailureCooldown
	}
	return LoginFailureUnknown, true
}

func cloneLoginFailure(failure *LoginFailure) *LoginFailure {
	if failure == nil {
		return nil
	}
	cloned := *failure
	if failure.SuspendedUntil != nil {
		until := *failure.SuspendedUntil
		cloned.SuspendedUntil = &until
	}
	return &cloned
}

type AccountBindingState struct {
	UID      int64     `json:"uid,omitempty"`
	WorldID  string    `json:"worldId,omitempty"`
	PlayerID PlayerID  `json:"playerId,omitempty"`
	BoundAt  time.Time `json:"boundAt,omitempty"`
}

type PlayerState struct {
	ID                PlayerID                  `json:"id"`
	Name              string                    `json:"name,omitempty"`
	AllianceID        AllianceID                `json:"allianceId,omitempty"`
	Level             int                       `json:"level,omitempty"`
	LegendLevel       int                       `json:"legendLevel,omitempty"`
	Might             float64                   `json:"might,omitempty"`
	Glory             float64                   `json:"glory,omitempty"`
	GloryTitleID      int64                     `json:"gloryTitleId,omitempty"`
	GloryTitleTopX    int                       `json:"gloryTitleTopX,omitempty"`
	GloryTitleAt      time.Time                 `json:"gloryTitleObservedAt,omitempty"`
	GloryTitleGen     uint64                    `json:"gloryTitleGeneration,omitempty"`
	Gallantry         float64                   `json:"gallantry,omitempty"`
	GallantryTitleID  int64                     `json:"gallantryTitleId,omitempty"`
	GallantryTitleAt  time.Time                 `json:"gallantryTitleObservedAt,omitempty"`
	GallantryTitleGen uint64                    `json:"gallantryTitleGeneration,omitempty"`
	Resources         map[ResourceID]float64    `json:"resources"`
	Currencies        map[CurrencyID]float64    `json:"currencies"`
	VIP               VIPState                  `json:"vip"`
	ProtectionMode    PlayerProtectionModeState `json:"protectionMode"`
	Achievements      AchievementState          `json:"achievements"`
	LegendSkills      LegendSkillState          `json:"legendSkills"`
}

// CurrentGloryTitle returns a title only when it was observed on the active
// game connection. This prevents a persisted title from authorizing a recruit
// after reconnecting into a newer game state.
func (player PlayerState) CurrentGloryTitle(connectionGeneration uint64) (int64, bool) {
	return currentPlayerTitle(player.GloryTitleID, player.GloryTitleAt, player.GloryTitleGen, connectionGeneration)
}

// CurrentGallantryTitle returns the separately reported FACTION title from
// the active connection. Gallantry title updates never authorize recruitment.
func (player PlayerState) CurrentGallantryTitle(connectionGeneration uint64) (int64, bool) {
	return currentPlayerTitle(player.GallantryTitleID, player.GallantryTitleAt, player.GallantryTitleGen, connectionGeneration)
}

func currentPlayerTitle(titleID int64, observedAt time.Time, observedGeneration uint64, connectionGeneration uint64) (int64, bool) {
	if observedAt.IsZero() || titleID < 0 {
		return 0, false
	}
	if connectionGeneration > 0 && observedGeneration != connectionGeneration {
		return 0, false
	}
	return titleID, true
}

type PlayerProtectionModeState struct {
	ModeState      int       `json:"modeState"`
	ModeTimerSec   int64     `json:"modeTimerSec,omitempty"`
	ModeObservedAt time.Time `json:"modeObservedAt,omitempty"`
	RemainingSec   int64     `json:"remainingSec"`
	ObservedAt     time.Time `json:"observedAt,omitempty"`
}

// PreparingOrActive is based on the player's game-reported RPT countdown.
// PMT is state-dependent and can remain positive as a repurchase cooldown after
// Protection Mode is canceled, so it must not be used as protection time.
func (state PlayerProtectionModeState) PreparingOrActive(now time.Time) bool {
	return !now.IsZero() && now.Before(state.Until())
}

func (state PlayerProtectionModeState) Until() time.Time {
	if state.ObservedAt.IsZero() || state.RemainingSec <= 0 {
		return time.Time{}
	}
	return state.ObservedAt.Add(time.Duration(state.RemainingSec) * time.Second)
}

type AchievementState struct {
	Points     int64             `json:"points"`
	Completed  map[int64]bool    `json:"completed"`
	Progress   map[int64][]int64 `json:"progress"`
	ObservedAt time.Time         `json:"observedAt,omitempty"`
}

type LegendSkillState struct {
	ActiveIDs         []int64                `json:"activeIds"`
	SkillPoints       int64                  `json:"skillPoints"`
	ResetRemainingSec int64                  `json:"resetRemainingSec"`
	ResetCount        int64                  `json:"resetCount"`
	SceatSkillIDs     []int64                `json:"sceatSkillIds"`
	SceatActivations  []SceatSkillActivation `json:"sceatActivations"`
	ObservedAt        time.Time              `json:"observedAt,omitempty"`
}

type SceatSkillActivation struct {
	ID           int64 `json:"id"`
	RemainingSec int64 `json:"remainingSec"`
}

type VIPState struct {
	Points       int64 `json:"points,omitempty"`
	Level        int   `json:"level,omitempty"`
	RemainingSec int   `json:"remainingSec,omitempty"`
	Upgrade      int   `json:"upgrade,omitempty"`
}

type CastleState struct {
	ID                          CastleID                                  `json:"id"`
	KingdomID                   KingdomID                                 `json:"kingdomId"`
	SlotType                    int                                       `json:"slotType,omitempty"`
	Name                        string                                    `json:"name,omitempty"`
	X                           int                                       `json:"x"`
	Y                           int                                       `json:"y"`
	Focused                     bool                                      `json:"focused"`
	Resources                   map[ResourceID]ResourceBalance            `json:"resources"`
	FoodStateObservedAt         time.Time                                 `json:"foodStateObservedAt,omitempty"`
	Units                       CastleUnits                               `json:"units"`
	UnitsObservedAt             time.Time                                 `json:"unitsObservedAt,omitempty"`
	Defense                     CastleDefenseState                        `json:"defense"`
	Buildings                   map[BuildingInstanceID]Building           `json:"buildings"`
	BuildingProduction          map[BuildingInstanceID]BuildingProduction `json:"buildingProduction"`
	Layout                      CastleLayout                              `json:"layout"`
	BuildingQueue               BuildingConstructionQueue                 `json:"buildingQueue"`
	ConstructionSlots           map[BuildingInstanceID][]ConstructionSlot `json:"constructionSlots"`
	ConstructionSlotsObservedAt time.Time                                 `json:"constructionSlotsObservedAt,omitempty"`
	Production                  map[int]ProductionQueue                   `json:"production"`
	QueueableProduction         map[int][]DefinitionRef                   `json:"queueableProduction"`
	QueueableObservedAt         time.Time                                 `json:"queueableObservedAt,omitempty"`
	Crafting                    CraftingState                             `json:"crafting"`
}

// SupportsSovereignCrafting identifies the four player castles where the
// sovereign-resource crafting buildings can exist. Other owned holdings are
// logistics/storage nodes even when they hold sovereign resources.
func (castle CastleState) SupportsSovereignCrafting() bool {
	if castle.KingdomID == 0 {
		return castle.SlotType == 1
	}
	return castle.KingdomID >= 1 && castle.KingdomID <= 3 && castle.SlotType == 12
}

type ResourceBalance struct {
	Amount                float64  `json:"amount"`
	ProductionPerHour     *float64 `json:"productionPerHour,omitempty"`
	ConsumptionPerHour    *float64 `json:"consumptionPerHour,omitempty"`
	ConsumptionMultiplier *float64 `json:"consumptionMultiplier,omitempty"`
	Capacity              *float64 `json:"capacity,omitempty"`
}

type CastleUnits struct {
	Stationed       map[UnitID]int64 `json:"stationed"`
	Traveling       map[UnitID]int64 `json:"traveling"`
	Hospital        map[UnitID]int64 `json:"hospital"`
	SpecialHospital map[UnitID]int64 `json:"specialHospital"`
	Total           map[UnitID]int64 `json:"total"`
}

type CastleDefenseState struct {
	Wall                DefenseWallState `json:"wall"`
	Keep                DefenseKeepState `json:"keep"`
	Moat                DefenseMoatState `json:"moat"`
	CastellanID         CastellanID      `json:"castellanId,omitempty"`
	GateDefense         float64          `json:"gateDefense,omitempty"`
	MeleeDefense        float64          `json:"meleeDefense,omitempty"`
	RangedDefense       float64          `json:"rangedDefense,omitempty"`
	HDWL                int64            `json:"hdwl,omitempty"`
	RangedUnitIDs       []UnitID         `json:"rangedUnitIds"`
	MeleeUnitIDs        []UnitID         `json:"meleeUnitIds"`
	Inventory           map[UnitID]int64 `json:"inventory"`
	InventoryObservedAt time.Time        `json:"inventoryObservedAt,omitempty"`
	ObservedAt          time.Time        `json:"observedAt,omitempty"`
	OpenGateUntil       *time.Time       `json:"openGateUntil,omitempty"`
}

type DefenseWallState struct {
	Left           DefenseWallSection `json:"left"`
	Middle         DefenseWallSection `json:"middle"`
	Right          DefenseWallSection `json:"right"`
	UnitCount      int64              `json:"unitCount,omitempty"`
	StationedUnits int64              `json:"stationedUnits,omitempty"`
	Defense        float64            `json:"defense,omitempty"`
	ObservedAt     time.Time          `json:"observedAt,omitempty"`
}

type DefenseWallSection struct {
	ToolSlots       []DefenseToolSlot `json:"toolSlots"`
	UnitPercent     int               `json:"unitPercent"`
	UnitTypePercent int               `json:"unitTypePercent"`
}

// MAUCT, UYL, and AUYL retain the game's field names until captures establish
// their exact semantics. S and STS retain their observed two-value slot shape.
// Official unit catalog slot types identify S as keep tools (5) and STS as
// Sceat defense-support tools (6).
type DefenseKeepState struct {
	PrimaryToolSlots   []DefenseToolSlot `json:"primaryToolSlots"`
	SecondaryToolSlots []DefenseToolSlot `json:"secondaryToolSlots"`
	MAUCT              int64             `json:"mauct,omitempty"`
	UnitTypePercent    int               `json:"unitTypePercent"`
	UnitCount          int64             `json:"unitCount,omitempty"`
	UYL                int64             `json:"uyl,omitempty"`
	AUYL               int64             `json:"auyl,omitempty"`
	ObservedAt         time.Time         `json:"observedAt,omitempty"`
}

type DefenseMoatState struct {
	LeftToolSlots   []DefenseToolSlot `json:"leftToolSlots"`
	MiddleToolSlots []DefenseToolSlot `json:"middleToolSlots"`
	RightToolSlots  []DefenseToolSlot `json:"rightToolSlots"`
	Defense         float64           `json:"defense,omitempty"`
	ObservedAt      time.Time         `json:"observedAt,omitempty"`
}

type DefenseToolSlot struct {
	DefinitionID UnitID `json:"definitionId"`
	Amount       int64  `json:"amount"`
}

type Building struct {
	InstanceID        BuildingInstanceID `json:"instanceId"`
	DefinitionID      BuildingID         `json:"definitionId"`
	GridX             int                `json:"gridX,omitempty"`
	GridY             int                `json:"gridY,omitempty"`
	Rotation          int                `json:"rotation,omitempty"`
	ProgressSec       int64              `json:"progressSec,omitempty"`
	ConstructionState int                `json:"constructionState,omitempty"`
	Level             int                `json:"level,omitempty"`
	Layer             BuildingLayer      `json:"layer,omitempty"`
	Placed            bool               `json:"placed"`
}

type BuildingProduction struct {
	PercentByResource map[string]float64 `json:"percentByResource"`
	ObservedAt        time.Time          `json:"observedAt,omitempty"`
}

const (
	BuildingStateInitial               = 0
	BuildingStateBuildStopped          = 1
	BuildingStateBuildInProgress       = 2
	BuildingStateBuildCompleted        = 4
	BuildingStateDisassembleStopped    = 5
	BuildingStateDisassembleInProgress = 6
	BuildingStateDisassembledCompleted = 8
	BuildingStateRepairStopped         = 9
	BuildingStateRepairInProgress      = 10
	BuildingStateUpgradeStopped        = 12
	BuildingStateUpgradeInProgress     = 13
	BuildingStateUpgradeCompleted      = 15
	BuildingStateWaitingForServer      = 100
)

type BuildingLayer string

const (
	BuildingLayerBG BuildingLayer = "BG"
	BuildingLayerBD BuildingLayer = "BD"
	BuildingLayerT  BuildingLayer = "T"
	BuildingLayerG  BuildingLayer = "G"
	BuildingLayerD  BuildingLayer = "D"
	BuildingLayerFP BuildingLayer = "FP"
)

type CastleLayout struct {
	Ground     map[BuildingInstanceID]Building `json:"ground"`
	Objects    map[BuildingInstanceID]Building `json:"objects"`
	Fixed      map[BuildingInstanceID]Building `json:"fixed"`
	ObservedAt time.Time                       `json:"observedAt,omitempty"`
}

type BuildingConstructionQueue struct {
	SlotCount  int                             `json:"slotCount"`
	Slots      []BuildingConstructionQueueSlot `json:"slots"`
	ObservedAt time.Time                       `json:"observedAt,omitempty"`
}

type BuildingConstructionQueueSlot struct {
	Index      int                `json:"index"`
	WireValue  int64              `json:"wireValue"`
	Status     string             `json:"status"`
	BuildingID BuildingInstanceID `json:"buildingId,omitempty"`
}

const (
	BuildingQueueSlotAvailable = "available"
	BuildingQueueSlotLocked    = "locked"
	BuildingQueueSlotOccupied  = "occupied"
	BuildingQueueSlotUnknown   = "unknown"
)

type ConstructionSlot struct {
	DefinitionID ConstructionItemID `json:"definitionId"`
	Slot         int                `json:"slot"`
	RemainingSec *int               `json:"remainingSec,omitempty"`
	Level        int                `json:"level,omitempty"`
}

type QueueItem struct {
	Definition            DefinitionRef `json:"definition"`
	Amount                int64         `json:"amount,omitempty"`
	StartedAt             *time.Time    `json:"startedAt,omitempty"`
	CompletesAt           *time.Time    `json:"completesAt,omitempty"`
	ProductionID          int64         `json:"productionId,omitempty"`
	AllianceHelpAvailable bool          `json:"allianceHelpAvailable,omitempty"`
	AllianceHelpRequested bool          `json:"allianceHelpRequested,omitempty"`
}

type ProductionQueue struct {
	LineID     int         `json:"lineId"`
	Active     *QueueItem  `json:"active,omitempty"`
	Queued     []QueueItem `json:"queued"`
	Capacity   int         `json:"capacity"`
	ObservedAt time.Time   `json:"observedAt"`
}

type AllianceHelpRequestState struct {
	HospitalProductionIDs    []int64    `json:"hospitalProductionIds"`
	RecruitmentCastleIDs     []CastleID `json:"recruitmentCastleIds"`
	PendingOtherListIDs      []int64    `json:"pendingOtherListIds"`
	ObservedAt               time.Time  `json:"observedAt,omitempty"`
	OthersObservedAt         time.Time  `json:"othersObservedAt,omitempty"`
	OthersObservedGeneration uint64     `json:"othersObservedGeneration,omitempty"`
	LastHelpAllAt            time.Time  `json:"lastHelpAllAt,omitempty"`
	LastHelpAllGeneration    uint64     `json:"lastHelpAllGeneration,omitempty"`
}

func PendingOtherAllianceHelpListIDs(state GameState) []int64 {
	requests := state.AllianceHelpRequests
	if state.Session.Generation == 0 || requests.OthersObservedGeneration != state.Session.Generation {
		return nil
	}
	result := make([]int64, 0, len(requests.PendingOtherListIDs))
	for _, listID := range requests.PendingOtherListIDs {
		if listID > 0 {
			result = append(result, listID)
		}
	}
	return result
}

func OutstandingHospitalAllianceHelpRequests(state GameState) int {
	if !state.AllianceHelpRequests.ObservedAt.IsZero() {
		productionIDs := map[int64]struct{}{}
		for _, productionID := range state.AllianceHelpRequests.HospitalProductionIDs {
			if productionID > 0 {
				productionIDs[productionID] = struct{}{}
			}
		}
		return len(productionIDs)
	}
	productionIDs := map[int64]struct{}{}
	for _, castle := range state.Castles {
		queue, exists := castle.Production[2]
		if !exists {
			continue
		}
		if queue.Active != nil && queue.Active.ProductionID > 0 && queue.Active.AllianceHelpRequested {
			productionIDs[queue.Active.ProductionID] = struct{}{}
		}
		for _, item := range queue.Queued {
			if item.ProductionID > 0 && item.AllianceHelpRequested {
				productionIDs[item.ProductionID] = struct{}{}
			}
		}
	}
	return len(productionIDs)
}

func HasOutstandingHospitalAllianceHelpRequest(state GameState, productionID int64) bool {
	if productionID <= 0 {
		return false
	}
	if !state.AllianceHelpRequests.ObservedAt.IsZero() {
		for _, outstandingID := range state.AllianceHelpRequests.HospitalProductionIDs {
			if outstandingID == productionID {
				return true
			}
		}
		return false
	}
	for _, castle := range state.Castles {
		queue, exists := castle.Production[2]
		if !exists {
			continue
		}
		if queue.Active != nil && queue.Active.ProductionID == productionID && queue.Active.AllianceHelpRequested {
			return true
		}
		for _, item := range queue.Queued {
			if item.ProductionID == productionID && item.AllianceHelpRequested {
				return true
			}
		}
	}
	return false
}

func HasOutstandingRecruitmentAllianceHelpRequest(state GameState, castleID CastleID) bool {
	if castleID <= 0 {
		return false
	}
	for _, outstandingID := range state.AllianceHelpRequests.RecruitmentCastleIDs {
		if outstandingID == castleID {
			return true
		}
	}
	if !state.AllianceHelpRequests.ObservedAt.IsZero() {
		return false
	}
	castle, exists := state.Castles[castleID]
	if !exists {
		return false
	}
	queue, exists := castle.Production[0]
	if !exists {
		return false
	}
	if queue.Active != nil && queue.Active.AllianceHelpRequested {
		return true
	}
	for _, item := range queue.Queued {
		if item.AllianceHelpRequested {
			return true
		}
	}
	return false
}

type CraftingState struct {
	Buildings              map[BuildingInstanceID]CraftingBuilding `json:"buildings"`
	EnabledRecipeIDs       []int64                                 `json:"enabledRecipeIds"`
	EnabledRecipeGroupIDs  []int64                                 `json:"enabledRecipeGroupIds"`
	OutputBoostByQueueType map[int]float64                         `json:"outputBoostByQueueType"`
}

type CraftingBuilding struct {
	KingdomID         KingdomID           `json:"kingdomId"`
	CastleID          CastleID            `json:"castleId"`
	InstanceID        BuildingInstanceID  `json:"instanceId"`
	DefinitionID      BuildingID          `json:"definitionId"`
	QueueTypeID       int                 `json:"queueTypeId"`
	SlotCount         int                 `json:"slotCount,omitempty"`
	ActiveSlotRentals []int               `json:"activeSlotRentals"`
	QueueSlotRentals  []int               `json:"queueSlotRentals"`
	Active            []CraftingQueueItem `json:"active"`
	Queued            []CraftingQueueItem `json:"queued"`
	ObservedAt        time.Time           `json:"observedAt"`
}

type CraftingQueueItem struct {
	RecipeID     int64   `json:"recipeId"`
	BatchValue   float64 `json:"batchValue,omitempty"`
	RemainingSec *int    `json:"remainingSec,omitempty"`
	RuntimeSec   *int    `json:"runtimeSec,omitempty"`
}

type CommanderState struct {
	ID              CommanderID                    `json:"id"`
	Name            string                         `json:"name,omitempty"`
	VisiblePosition int                            `json:"visiblePosition,omitempty"`
	Available       bool                           `json:"available"`
	GeneralID       int64                          `json:"generalId,omitempty"`
	Equipment       map[string]EquipmentInstanceID `json:"equipment"`
	Gems            map[string]GemInstanceID       `json:"gems"`
}

type GeneralState struct {
	ID             int64     `json:"id"`
	ActiveSkillIDs []int64   `json:"activeSkillIds"`
	ObservedAt     time.Time `json:"observedAt,omitempty"`
}

type CastellanState struct {
	ID        CastellanID                    `json:"id"`
	CastleID  CastleID                       `json:"castleId,omitempty"`
	Name      string                         `json:"name,omitempty"`
	Equipment map[string]EquipmentInstanceID `json:"equipment"`
	Gems      map[string]GemInstanceID       `json:"gems"`
}

type MovementState struct {
	ID              MovementID             `json:"id"`
	TypeID          int                    `json:"typeId,omitempty"`
	Direction       int                    `json:"direction"`
	OwnerPlayerID   PlayerID               `json:"ownerPlayerId,omitempty"`
	TargetPlayerID  PlayerID               `json:"targetPlayerId,omitempty"`
	SourceTypeID    int                    `json:"sourceTypeId,omitempty"`
	SourceCastleID  CastleID               `json:"sourceCastleId,omitempty"`
	TargetCastleID  CastleID               `json:"targetCastleId,omitempty"`
	TargetTypeID    int                    `json:"targetTypeId,omitempty"`
	CommanderID     *CommanderID           `json:"commanderId,omitempty"`
	KingdomID       KingdomID              `json:"kingdomId"`
	SourceX         int                    `json:"sourceX,omitempty"`
	SourceY         int                    `json:"sourceY,omitempty"`
	TargetX         int                    `json:"targetX"`
	TargetY         int                    `json:"targetY"`
	TravelSeconds   int                    `json:"travelSeconds,omitempty"`
	WaitSeconds     int                    `json:"waitSeconds,omitempty"`
	ProgressSeconds int                    `json:"progressSeconds,omitempty"`
	SpyCount        int                    `json:"spyCount,omitempty"`
	ArrivesAt       *time.Time             `json:"arrivesAt,omitempty"`
	ReturnsAt       *time.Time             `json:"returnsAt,omitempty"`
	StartedAt       time.Time              `json:"startedAt,omitempty"`
	ObservedAt      time.Time              `json:"observedAt,omitempty"`
	Units           map[UnitID]int64       `json:"units"`
	MarketBarrows   int                    `json:"marketBarrows,omitempty"`
	MarketGoods     []KingdomTransportGood `json:"marketGoods,omitempty"`
}

func (movement MovementState) ProjectedCompletionAt() *time.Time {
	if movement.Direction == 1 {
		if movement.ReturnsAt == nil {
			return nil
		}
		completion := movement.ReturnsAt.UTC()
		return &completion
	}
	if movement.Direction != 0 || movement.ArrivesAt == nil {
		return nil
	}
	completion := movement.ArrivesAt.UTC()
	return &completion
}

type EquipmentInstance struct {
	ID           EquipmentInstanceID `json:"id"`
	DefinitionID EquipmentID         `json:"definitionId"`
	Slot         int                 `json:"slot"`
	TypeID       int                 `json:"typeId,omitempty"`
	RarityID     int                 `json:"rarityId,omitempty"`
	SetID        int64               `json:"setId,omitempty"`
	Level        int                 `json:"level,omitempty"`
	WearerID     int64               `json:"wearerId,omitempty"`
	WearerKind   string              `json:"wearerKind,omitempty"`
	Effects      EquipmentEffects    `json:"effects"`
}

type EquipmentEffect struct {
	WireID       int64     `json:"wireId"`
	DefinitionID int64     `json:"definitionId"`
	RollPercent  *float64  `json:"rollPercent,omitempty"`
	Values       []float64 `json:"values"`
}

type EquipmentEffects []EquipmentEffect

func (effects *EquipmentEffects) UnmarshalJSON(raw []byte) error {
	var ordered []EquipmentEffect
	if err := json.Unmarshal(raw, &ordered); err == nil {
		*effects = ordered
		return nil
	}
	// Schema 2 snapshots written before ordered effects used an id-keyed map.
	// Accept it for migration; the next live equipment refresh restores exact
	// roll percentages and any duplicate rows that the old shape could not hold.
	var legacy map[string][]float64
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return err
	}
	ordered = make([]EquipmentEffect, 0, len(legacy))
	for rawID, values := range legacy {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			continue
		}
		ordered = append(ordered, EquipmentEffect{WireID: id, DefinitionID: id, Values: append([]float64(nil), values...)})
	}
	*effects = ordered
	return nil
}

type GemInstance struct {
	ID                  GemInstanceID       `json:"id"`
	DefinitionID        GemID               `json:"definitionId"`
	TypeID              int                 `json:"typeId,omitempty"`
	CompatibleWearerID  int                 `json:"compatibleWearerId,omitempty"`
	CombatMode          string              `json:"combatMode,omitempty"`
	SetID               int64               `json:"setId,omitempty"`
	Slot                int                 `json:"slot,omitempty"`
	Level               int                 `json:"level,omitempty"`
	EquipmentInstanceID EquipmentInstanceID `json:"equipmentInstanceId,omitempty"`
	WearerID            int64               `json:"wearerId,omitempty"`
	WearerKind          string              `json:"wearerKind,omitempty"`
	Effects             EquipmentEffects    `json:"effects"`
}

// ConstructionOfferSnapshot keeps one official GBC purchase-counter view
// bound to the castle and kingdom that produced it. The index is private
// runtime state: dashboard clients continue receiving only the most recently
// observed official response through InventoryState's legacy projection.
type ConstructionOfferSnapshot struct {
	Offers     map[PackageID]int64 `json:"offers"`
	ObservedAt time.Time           `json:"observedAt,omitempty"`
	CastleID   CastleID            `json:"castleId"`
	KingdomID  KingdomID           `json:"kingdomId"`
}

type InventoryState struct {
	ConstructionItems            map[ConstructionItemID]int64              `json:"constructionItems"`
	ConstructionItemsObservedAt  time.Time                                 `json:"constructionItemsObservedAt,omitempty"`
	ConstructionOffers           map[PackageID]int64                       `json:"constructionOffers"`
	ConstructionOffersObservedAt time.Time                                 `json:"constructionOffersObservedAt,omitempty"`
	ConstructionOffersCastleID   CastleID                                  `json:"constructionOffersCastleId,omitempty"`
	ConstructionOffersKingdomID  KingdomID                                 `json:"constructionOffersKingdomId"`
	ConstructionOffersByCastle   map[CastleID]ConstructionOfferSnapshot    `json:"-"`
	Equipment                    map[EquipmentInstanceID]EquipmentInstance `json:"equipment"`
	Gems                         map[GemInstanceID]GemInstance             `json:"gems"`
	GemStacks                    map[GemID]int64                           `json:"gemStacks"`
	Items                        map[string]map[int64]int64                `json:"items"`
}

type SubscriptionState struct {
	TypeID         int `json:"typeId"`
	RemainingSec   int `json:"remainingSec,omitempty"`
	GracePeriodSec int `json:"gracePeriodSec,omitempty"`
}

type MarketAreaEffect struct {
	EffectID int64     `json:"effectId"`
	Values   []float64 `json:"values"`
	Source   string    `json:"source,omitempty"`
}

type MarketCastleState struct {
	CastleID         CastleID               `json:"castleId"`
	KingdomID        KingdomID              `json:"kingdomId"`
	TotalBarrows     int                    `json:"totalBarrows"`
	AvailableBarrows int                    `json:"availableBarrows"`
	Resources        map[ResourceID]float64 `json:"resources"`
	AreaEffects      []MarketAreaEffect     `json:"areaEffects"`
}

type MarketBoosterState struct {
	ID                      int       `json:"id"`
	Level                   int       `json:"level,omitempty"`
	BonusPercent            int       `json:"bonusPercent,omitempty"`
	RemainingSec            int       `json:"remainingSec,omitempty"`
	ContinuousPurchaseCount int       `json:"continuousPurchaseCount,omitempty"`
	ExpiresAt               time.Time `json:"expiresAt,omitempty"`
	Permanent               bool      `json:"permanent,omitempty"`
}

func (booster MarketBoosterState) ActiveAt(now time.Time) bool {
	if booster.Permanent {
		return booster.Level > 0 || booster.BonusPercent > 0
	}
	return !booster.ExpiresAt.IsZero() && booster.ExpiresAt.After(now)
}

type MarketFeastState struct {
	ID           int64     `json:"id,omitempty"`
	RemainingSec int       `json:"remainingSec,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	ObservedAt   time.Time `json:"observedAt,omitempty"`
}

func (feast MarketFeastState) ActiveAt(now time.Time) bool {
	return feast.ID >= 0 && !feast.ExpiresAt.IsZero() && feast.ExpiresAt.After(now)
}

type MarketState struct {
	Castles            map[CastleID]MarketCastleState `json:"castles"`
	Boosters           map[int]MarketBoosterState     `json:"boosters"`
	Feast              MarketFeastState               `json:"feast"`
	CaravanLevel       int                            `json:"caravanLevel,omitempty"`
	CaravanLevelLoaded bool                           `json:"caravanLevelLoaded"`
	ObservedAt         time.Time                      `json:"observedAt,omitempty"`
	BoostersObservedAt time.Time                      `json:"boostersObservedAt,omitempty"`
}

type KingdomTransportUnlock struct {
	KingdomID KingdomID `json:"kingdomId"`
	Unlocked  bool      `json:"unlocked"`
	Created   bool      `json:"created"`
	Stage     int       `json:"stage,omitempty"`
}

type KingdomTransportGood struct {
	ResourceID ResourceID `json:"resourceId"`
	Amount     float64    `json:"amount"`
}

type KingdomResourceTransport struct {
	KingdomID    KingdomID              `json:"kingdomId"`
	RemainingSec int                    `json:"remainingSec,omitempty"`
	Goods        []KingdomTransportGood `json:"goods"`
}

type KingdomResourceTransportWorkflow struct {
	Owner          string                 `json:"owner"`
	KingdomID      KingdomID              `json:"kingdomId"`
	SourceCastleID CastleID               `json:"sourceCastleId"`
	TargetCastleID CastleID               `json:"targetCastleId"`
	Goods          []KingdomTransportGood `json:"goods"`
	LaunchedAt     time.Time              `json:"launchedAt"`
}

type KingdomTransportUnit struct {
	UnitID UnitID `json:"unitId"`
	Amount int64  `json:"amount"`
}

type KingdomUnitTransport struct {
	KingdomID    KingdomID              `json:"kingdomId"`
	RemainingSec int                    `json:"remainingSec,omitempty"`
	Units        []KingdomTransportUnit `json:"units"`
}

type KingdomTransportState struct {
	Unlocks           map[KingdomID]KingdomTransportUnlock           `json:"unlocks"`
	Pending           []KingdomResourceTransport                     `json:"pending"`
	PendingUnits      []KingdomUnitTransport                         `json:"pendingUnits"`
	ResourceWorkflows map[KingdomID]KingdomResourceTransportWorkflow `json:"resourceWorkflows,omitempty"`
	ObservedAt        time.Time                                      `json:"observedAt,omitempty"`
}

type BeriState struct {
	AvailableTroops     int64            `json:"availableTroops"`
	TroopsByUnit        map[UnitID]int64 `json:"troopsByUnit"`
	ParsedSourceID      CastleID         `json:"parsedSourceId,omitempty"`
	ObservedAt          time.Time        `json:"observedAt,omitempty"`
	ConsumedAt          time.Time        `json:"consumedAt,omitempty"`
	CampOpenRequestedAt time.Time        `json:"campOpenRequestedAt,omitempty"`
	TargetX             int              `json:"targetX,omitempty"`
	TargetY             int              `json:"targetY,omitempty"`
	TargetTypeID        int              `json:"targetTypeId,omitempty"`
	TargetObservedAt    time.Time        `json:"targetObservedAt,omitempty"`
	TargetConsumedAt    time.Time        `json:"targetConsumedAt,omitempty"`
	TargetInvalidatedAt time.Time        `json:"targetInvalidatedAt,omitempty"`
}

type AllianceMember struct {
	PlayerID            PlayerID `json:"playerId"`
	Name                string   `json:"name,omitempty"`
	RankID              int      `json:"rankId,omitempty"`
	Level               int      `json:"level,omitempty"`
	LegendLevel         int      `json:"legendLevel,omitempty"`
	Might               float64  `json:"might,omitempty"`
	ReturnProtectionSec int      `json:"returnProtectionSec,omitempty"`
}

type AllianceHolding struct {
	CastleID  CastleID  `json:"castleId"`
	PlayerID  PlayerID  `json:"playerId"`
	KingdomID KingdomID `json:"kingdomId"`
	X         int       `json:"x"`
	Y         int       `json:"y"`
	SlotType  int       `json:"slotType"`
}

type AllianceState struct {
	ID         AllianceID        `json:"id"`
	Name       string            `json:"name,omitempty"`
	Members    []AllianceMember  `json:"members"`
	Holdings   []AllianceHolding `json:"holdings"`
	ObservedAt time.Time         `json:"observedAt,omitempty"`
}

type MovementSnapshot struct {
	Version              uint64    `json:"version"`
	ConnectionGeneration uint64    `json:"connectionGeneration,omitempty"`
	ObservedAt           time.Time `json:"observedAt,omitempty"`
}

type StationingPhase string

const (
	StationingPhaseTargetReady   StationingPhase = "target-ready"
	StationingPhaseDispatchReady StationingPhase = "dispatch-ready"
	StationingPhaseAway          StationingPhase = "away"
	StationingPhaseWaiting       StationingPhase = "waiting"
)

type StationingOperation struct {
	ID                   string           `json:"id"`
	Purpose              string           `json:"purpose"`
	Phase                StationingPhase  `json:"phase,omitempty"`
	SourceCastleID       CastleID         `json:"sourceCastleId"`
	TargetCastleID       CastleID         `json:"targetCastleId"`
	MovementID           MovementID       `json:"movementId,omitempty"`
	Units                map[UnitID]int64 `json:"units"`
	DelayHours           int              `json:"delayHours,omitempty"`
	WaitSeconds          int              `json:"waitSeconds,omitempty"`
	TravelSeconds        int              `json:"travelSeconds,omitempty"`
	AllianceObservedAt   time.Time        `json:"allianceObservedAt,omitempty"`
	UnitsObservedAt      time.Time        `json:"unitsObservedAt,omitempty"`
	DispatchedAt         *time.Time       `json:"dispatchedAt,omitempty"`
	ExpectedReturnAt     *time.Time       `json:"expectedReturnAt,omitempty"`
	NextAttemptAt        *time.Time       `json:"nextAttemptAt,omitempty"`
	StatusDetail         string           `json:"statusDetail,omitempty"`
	SafeAfter            *time.Time       `json:"safeAfter,omitempty"`
	SuccessCooldownUntil *time.Time       `json:"successCooldownUntil,omitempty"`
	CreatedAt            time.Time        `json:"createdAt"`
	UpdatedAt            time.Time        `json:"updatedAt"`
}

func (operation StationingOperation) MatchesMovement(movement MovementState) bool {
	if operation.MovementID > 0 {
		return movement.ID == operation.MovementID
	}
	if movement.Direction == 1 {
		return movement.SourceCastleID == operation.TargetCastleID &&
			movement.TargetCastleID == operation.SourceCastleID
	}
	return movement.SourceCastleID == operation.SourceCastleID &&
		movement.TargetCastleID == operation.TargetCastleID
}

func (operation StationingOperation) ActiveAt(movements map[MovementID]MovementState, now time.Time) bool {
	for _, movement := range movements {
		if operation.MatchesMovement(movement) && StationMovementActiveAt(movement, now) {
			return true
		}
	}
	if operation.MovementID > 0 {
		return false
	}
	if operation.SuccessCooldownUntil != nil && operation.SuccessCooldownUntil.After(now) {
		return true
	}
	return !operation.UpdatedAt.IsZero() && now.Before(operation.UpdatedAt.Add(30*time.Second))
}

type ScheduledOperation struct {
	ID              string          `json:"id"`
	Version         uint64          `json:"version"`
	Intent          string          `json:"intent"`
	Actor           string          `json:"actor"`
	Arguments       json.RawMessage `json:"arguments"`
	WorldID         string          `json:"worldId,omitempty"`
	PlayerID        PlayerID        `json:"playerId,omitempty"`
	ExecuteAt       time.Time       `json:"executeAt"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Status          string          `json:"status"`
	LastOperationID string          `json:"lastOperationId,omitempty"`
	LastError       string          `json:"lastError,omitempty"`
}

type RiftLaunch struct {
	ID                string          `json:"id"`
	DisplayName       string          `json:"displayName,omitempty"`
	SavedAtUnix       int64           `json:"savedAtUnix"`
	Body              json.RawMessage `json:"body"`
	CommanderID       CommanderID     `json:"commanderID,omitempty"`
	SourceX           int             `json:"sourceX,omitempty"`
	SourceY           int             `json:"sourceY,omitempty"`
	TargetX           int             `json:"targetX,omitempty"`
	TargetY           int             `json:"targetY,omitempty"`
	KingdomID         KingdomID       `json:"kingdomID,omitempty"`
	AttackValid       int             `json:"attackValid,omitempty"`
	WaveCount         int             `json:"waveCount,omitempty"`
	UseTravelFeather  bool            `json:"useTravelFeather"`
	OneWayTTSeconds   int             `json:"oneWayTTSeconds,omitempty"`
	LastSuccessAtUnix int64           `json:"lastSuccessAtUnix,omitempty"`
}

type RiftMaidenRunState struct {
	ID                 string        `json:"id"`
	Status             string        `json:"status"`
	RequestedAttacks   int           `json:"requestedAttacks"`
	AttacksLaunched    int           `json:"attacksLaunched"`
	UnitID             UnitID        `json:"unitId"`
	HorseTravelBoostID int           `json:"horseTravelBoostId"`
	CommanderIDs       []CommanderID `json:"commanderIds"`
	LaunchIDs          []MovementID  `json:"launchIds,omitempty"`
	SourceCastleID     CastleID      `json:"sourceCastleId"`
	SourceX            int           `json:"sourceX"`
	SourceY            int           `json:"sourceY"`
	KingdomID          KingdomID     `json:"kingdomId"`
	TargetX            int           `json:"targetX"`
	TargetY            int           `json:"targetY"`
	StartedAt          time.Time     `json:"startedAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type RiftState struct {
	Launches         map[string]RiftLaunch `json:"launches"`
	DeletedLaunchIDs map[string]int64      `json:"deletedLaunchIds,omitempty"`
	PendingLaunchID  string                `json:"pendingLaunchId,omitempty"`
	MaidenRun        *RiftMaidenRunState   `json:"maidenRun,omitempty"`
}

type MapObservation struct {
	KingdomID                  KingdomID `json:"kingdomId"`
	X                          int       `json:"x"`
	Y                          int       `json:"y"`
	TypeID                     int       `json:"typeId"`
	Name                       string    `json:"name,omitempty"`
	Level                      int       `json:"level,omitempty"`
	OwnerID                    PlayerID  `json:"ownerId,omitempty"`
	ObjectID                   int64     `json:"objectId,omitempty"`
	TowerVictoryCount          int64     `json:"towerVictoryCount,omitempty"`
	TowerCooldownRemaining     int       `json:"towerCooldownRemaining,omitempty"`
	EventCampID                int64     `json:"eventCampId,omitempty"`
	EventCampVictoryCount      int64     `json:"eventCampVictoryCount,omitempty"`
	EventCampCooldownRemaining int       `json:"eventCampCooldownRemaining,omitempty"`
	EventCampBaseWallBonus     int64     `json:"eventCampBaseWallBonus,omitempty"`
	EventCampBaseGateBonus     int64     `json:"eventCampBaseGateBonus,omitempty"`
	EventCampBaseMoatBonus     int64     `json:"eventCampBaseMoatBonus,omitempty"`
	StormIsleID                int64     `json:"stormIsleId,omitempty"`
	StormVictoryCount          int64     `json:"stormVictoryCount,omitempty"`
	StormCooldownRemaining     int       `json:"stormCooldownRemaining,omitempty"`
	ObservedAt                 time.Time `json:"observedAt"`
}

func (observation *MapObservation) UnmarshalJSON(raw []byte) error {
	type mapObservationAlias MapObservation
	var decoded mapObservationAlias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*observation = MapObservation(decoded)
	if observation.TowerVictoryCount != 0 {
		return nil
	}
	var legacy struct {
		TowerVictoryCount int64 `json:"towerBaseFlankCapacity"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return err
	}
	observation.TowerVictoryCount = legacy.TowerVictoryCount
	return nil
}

// StormReadyAt and StormExpiresAt are deterministic projections of the
// official map fields. Keeping them out of GameState saves two time.Time
// values on every map record and prevents reducers from persisting duplicate
// derived facts.
func (observation MapObservation) StormReadyAt() time.Time {
	if observation.ObservedAt.IsZero() {
		return time.Time{}
	}
	readyAt := observation.ObservedAt
	if observation.StormCooldownRemaining > 0 &&
		(observation.TypeID == MapTypeStormFort || observation.TypeID == MapTypeStormIsland && observation.OwnerID > 0) {
		readyAt = readyAt.Add(time.Duration(observation.StormCooldownRemaining) * time.Second)
	}
	return readyAt
}

func (observation MapObservation) StormExpiresAt(globalCooldownSec int64) time.Time {
	if observation.TypeID != MapTypeStormIsland || observation.ObservedAt.IsZero() {
		return time.Time{}
	}
	if observation.OwnerID <= 0 && observation.StormCooldownRemaining > 0 {
		return observation.ObservedAt.Add(time.Duration(observation.StormCooldownRemaining) * time.Second)
	}
	readyAt := observation.StormReadyAt()
	if observation.OwnerID > 0 && !readyAt.IsZero() && globalCooldownSec > 0 {
		return readyAt.Add(time.Duration(globalCooldownSec) * time.Second)
	}
	return time.Time{}
}

// TowerCooldownState records a confirmed successful tower battle and the
// cooldown returned by the following map refresh. A CRA acknowledgement never
// creates this state because it only confirms the troop movement was started.
type TowerCooldownState struct {
	KingdomID              KingdomID `json:"kingdomId"`
	X                      int       `json:"x"`
	Y                      int       `json:"y"`
	ReportID               int64     `json:"reportId,omitempty"`
	LastSuccessfulBattleAt time.Time `json:"lastSuccessfulBattleAt"`
	CooldownRemaining      int       `json:"cooldownRemaining,omitempty"`
	CooldownObservedAt     time.Time `json:"cooldownObservedAt,omitempty"`
	PendingCooldownRefresh bool      `json:"pendingCooldownRefresh,omitempty"`
}

// TowerQueueState stores the fresh, per-castle batch selected during a tower
// map scan. Entries are consumed only after their CRA succeeds, allowing the
// current castle to remain focused while the batch drains.
type TowerQueueEntry struct {
	KingdomID     KingdomID  `json:"kingdomId"`
	TargetX       int        `json:"targetX"`
	TargetY       int        `json:"targetY"`
	MapObservedAt time.Time  `json:"mapObservedAt"`
	QueuedAt      time.Time  `json:"queuedAt"`
	DeferredUntil *time.Time `json:"deferredUntil,omitempty"`
}

type TowerCapacityObservation struct {
	AdditionalUnits int64     `json:"additionalUnits"`
	FullFlankUnits  int64     `json:"fullFlankUnits"`
	ObservedAt      time.Time `json:"observedAt"`
}

type TowerQueueState struct {
	EntriesByCastle           map[CastleID][]TowerQueueEntry        `json:"entriesByCastle"`
	LastScannedAt             map[CastleID]time.Time                `json:"lastScannedAt"`
	LastAttemptedAt           map[CastleID]time.Time                `json:"lastAttemptedAt"`
	ConfirmedLaunchesByCastle map[CastleID]int64                    `json:"confirmedLaunchesByCastle"`
	CursorVersion             int                                   `json:"cursorVersion"`
	CapacityByCastle          map[CastleID]TowerCapacityObservation `json:"capacityByCastle"`
}

const TowerQueueCursorVersion = 1

type InvasionState struct {
	LastScannedAt        map[CastleID]time.Time `json:"lastScannedAt"`
	FortifiedTargets     map[string]string      `json:"fortifiedTargets"`
	FortifyCurrencies    []string               `json:"fortifyCurrencies"`
	FortifyResourceCount int64                  `json:"fortifyResourceCount"`
	FortifyRubyCount     int64                  `json:"fortifyRubyCount"`
}

func (state InvasionState) SupportsFortifyCurrency(currency string) bool {
	for _, supported := range state.FortifyCurrencies {
		if supported == currency {
			return true
		}
	}
	return false
}

type StormMapBounds struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

func (bounds StormMapBounds) IsValid() bool {
	return bounds.X1 >= 0 && bounds.Y1 >= 0 && bounds.X2 >= bounds.X1 && bounds.Y2 >= bounds.Y1
}

func (bounds StormMapBounds) Contains(x int, y int) bool {
	return bounds.IsValid() && x >= bounds.X1 && x <= bounds.X2 && y >= bounds.Y1 && y <= bounds.Y2
}

// StormMapState is the last authoritative, fully completed Storm sweep. Full
// scans restart at the fixed center and expand in square rings until every
// learned Storm target has a safe margin from the covered edge.
type StormMapState struct {
	ServerURL       string                    `json:"serverUrl,omitempty"`
	PlayerID        PlayerID                  `json:"playerId,omitempty"`
	SourceCastleID  CastleID                  `json:"sourceCastleId,omitempty"`
	CoveredBounds   StormMapBounds            `json:"coveredBounds"`
	NextBounds      StormMapBounds            `json:"nextBounds"`
	LastAttemptAt   time.Time                 `json:"lastAttemptAt,omitempty"`
	LastCompletedAt time.Time                 `json:"lastCompletedAt,omitempty"`
	WindowCount     int                       `json:"windowCount,omitempty"`
	Targets         map[string]MapObservation `json:"targets"`
	// suppressedTargets is account-private negative membership restored from
	// component persistence. It never appears in the official logical model or
	// crosses the account API boundary.
	suppressedTargets []string
}

const (
	StormIslandReturnAwaitingReport = "awaiting_report"
	StormIslandReturnReady          = "ready"
)

// StormIslandReturnState bridges an acknowledged island attack to the battle
// report that authoritatively identifies the surviving troops now occupying
// the island. Those survivors can then be returned without relying on the
// original attack formation.
type StormIslandReturnState struct {
	KingdomID      KingdomID        `json:"kingdomId"`
	SourceCastleID CastleID         `json:"sourceCastleId"`
	TargetX        int              `json:"targetX"`
	TargetY        int              `json:"targetY"`
	IslandObjectID int64            `json:"islandObjectId"`
	ReportID       int64            `json:"reportId,omitempty"`
	Status         string           `json:"status"`
	LeaveBehind    int64            `json:"leaveBehind"`
	Survivors      map[UnitID]int64 `json:"survivors"`
	LaunchedAt     time.Time        `json:"launchedAt"`
	ReportedAt     time.Time        `json:"reportedAt,omitempty"`
}

func StormIslandReturnKey(kingdomID KingdomID, x int, y int) string {
	return strconv.FormatInt(int64(kingdomID), 10) + ":" + strconv.Itoa(x) + ":" + strconv.Itoa(y)
}

// UnitsToReturn leaves the requested number of occupiers behind, taking them
// from the largest surviving stack first so a one-unit stack can still return
// when a larger stack is available.
func (state StormIslandReturnState) UnitsToReturn() map[UnitID]int64 {
	result := make(map[UnitID]int64, len(state.Survivors))
	for unitID, amount := range state.Survivors {
		if unitID > 0 && amount > 0 {
			result[unitID] = amount
		}
	}
	for remaining := state.LeaveBehind; remaining > 0 && len(result) > 0; {
		selectedID := UnitID(0)
		selectedAmount := int64(0)
		for unitID, amount := range result {
			if amount > selectedAmount || amount == selectedAmount && (selectedID == 0 || unitID < selectedID) {
				selectedID = unitID
				selectedAmount = amount
			}
		}
		if selectedID <= 0 || selectedAmount <= 0 {
			break
		}
		remove := remaining
		if remove > selectedAmount {
			remove = selectedAmount
		}
		if selectedAmount == remove {
			delete(result, selectedID)
		} else {
			result[selectedID] = selectedAmount - remove
		}
		remaining -= remove
	}
	return result
}

type StormState struct {
	LastScannedAt                 map[CastleID]time.Time            `json:"lastScannedAt"`
	Map                           StormMapState                     `json:"map"`
	IslandReturns                 map[string]StormIslandReturnState `json:"islandReturns"`
	LunaShopTableID               int64                             `json:"lunaShopTableId,omitempty"`
	LunaShopProductID             PackageID                         `json:"lunaShopProductId,omitempty"`
	LunaShopObservedAt            time.Time                         `json:"lunaShopObservedAt,omitempty"`
	LunaShopPending               bool                              `json:"lunaShopPending,omitempty"`
	LunaShopPendingCastleID       CastleID                          `json:"lunaShopPendingCastleId,omitempty"`
	LunaShopPendingAmount         int64                             `json:"lunaShopPendingAmount,omitempty"`
	LunaShopPendingAquamarineCost int64                             `json:"lunaShopPendingAquamarineCost,omitempty"`
}

type NomadCampTargetState struct {
	SourceCastleID CastleID  `json:"sourceCastleId"`
	EventID        int64     `json:"eventId"`
	DifficultyID   int64     `json:"difficultyId"`
	KingdomID      KingdomID `json:"kingdomId"`
	TypeID         int       `json:"typeId"`
	X              int       `json:"x"`
	Y              int       `json:"y"`
	EventCampID    int64     `json:"eventCampId"`
	VictoryCount   int64     `json:"victoryCount"`
	DefenseScore   int64     `json:"defenseScore"`
	EventEndsAt    time.Time `json:"eventEndsAt,omitempty"`
	LockedAt       time.Time `json:"lockedAt"`
}

type NomadCampCooldownState struct {
	KingdomID              KingdomID `json:"kingdomId"`
	X                      int       `json:"x"`
	Y                      int       `json:"y"`
	ReportID               int64     `json:"reportId,omitempty"`
	LastSuccessfulBattleAt time.Time `json:"lastSuccessfulBattleAt"`
	CooldownRemaining      int       `json:"cooldownRemaining,omitempty"`
	CooldownObservedAt     time.Time `json:"cooldownObservedAt,omitempty"`
	PendingCooldownRefresh bool      `json:"pendingCooldownRefresh,omitempty"`
}

type NomadRBCTestLaunch struct {
	BatchID     string      `json:"batchId,omitempty"`
	CommanderID CommanderID `json:"commanderId"`
	MovementID  MovementID  `json:"movementId"`
	ArrivesAt   time.Time   `json:"arrivesAt"`
}

type NomadRBCTestState struct {
	RunID                 string               `json:"runId"`
	SourceCastleID        CastleID             `json:"sourceCastleId"`
	KingdomID             KingdomID            `json:"kingdomId"`
	TargetX               int                  `json:"targetX"`
	TargetY               int                  `json:"targetY"`
	ExpectedAttacks       int                  `json:"expectedAttacks"`
	AttacksLaunched       int                  `json:"attacksLaunched"`
	VictoriesConfirmed    int                  `json:"victoriesConfirmed"`
	CooldownsSkipped      int                  `json:"cooldownsSkipped"`
	Launches              []NomadRBCTestLaunch `json:"launches"`
	LastReportID          int64                `json:"lastReportId,omitempty"`
	SafetyError           string               `json:"safetyError,omitempty"`
	StartedAt             time.Time            `json:"startedAt"`
	LastChainLaunchedAt   time.Time            `json:"lastChainLaunchedAt,omitempty"`
	LastCooldownSkippedAt time.Time            `json:"lastCooldownSkippedAt,omitempty"`
}

type NomadCampState struct {
	LastScannedAt map[CastleID]time.Time            `json:"lastScannedAt"`
	LockedTarget  *NomadCampTargetState             `json:"lockedTarget,omitempty"`
	Cooldowns     map[string]NomadCampCooldownState `json:"cooldowns"`
	RBCTest       *NomadRBCTestState                `json:"rbcTest,omitempty"`
}

const AdvisorEstimatedCycleSeconds = 130

type AdvisorRunState struct {
	EventID          int64       `json:"eventId"`
	EventEndsAt      time.Time   `json:"eventEndsAt,omitempty"`
	SourceCastleID   CastleID    `json:"sourceCastleId,omitempty"`
	KingdomID        KingdomID   `json:"kingdomId,omitempty"`
	TargetTypeID     int         `json:"targetTypeId,omitempty"`
	TargetX          int         `json:"targetX,omitempty"`
	TargetY          int         `json:"targetY,omitempty"`
	CommanderID      CommanderID `json:"commanderId,omitempty"`
	MovementID       MovementID  `json:"movementId,omitempty"`
	RequestedAttacks int         `json:"requestedAttacks"`
	CurrentAttack    int         `json:"currentAttack"`
	LaunchState      int         `json:"launchState,omitempty"`
	Status           string      `json:"status"`
	StartedAt        time.Time   `json:"startedAt"`
	LastAttackAt     time.Time   `json:"lastAttackAt,omitempty"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

type AdvisorSummaryState struct {
	AdvisorType    int              `json:"advisorType,omitempty"`
	Count          int              `json:"count,omitempty"`
	Gains          map[string]int64 `json:"gains"`
	Costs          map[string]int64 `json:"costs"`
	UnitsLost      int64            `json:"unitsLost,omitempty"`
	ToolsLost      int64            `json:"toolsLost,omitempty"`
	Wins           int64            `json:"wins,omitempty"`
	Defeats        int64            `json:"defeats,omitempty"`
	PendingAttacks int64            `json:"pendingAttacks,omitempty"`
	ObservedAt     time.Time        `json:"observedAt,omitempty"`
}

type AdvisorState struct {
	Run     *AdvisorRunState    `json:"run,omitempty"`
	Summary AdvisorSummaryState `json:"summary"`
}

type KhanLaunchState struct {
	CommanderID CommanderID `json:"commanderId"`
	MovementID  MovementID  `json:"movementId"`
	ArrivesAt   time.Time   `json:"arrivesAt,omitempty"`
}

type KhanTauntState struct {
	MovementID MovementID `json:"movementId"`
	ObservedAt time.Time  `json:"observedAt"`
	ImpactAt   time.Time  `json:"impactAt"`
}

type KhanProtectionState struct {
	Active                 bool      `json:"active"`
	CastleID               CastleID  `json:"castleId,omitempty"`
	OffensiveWallUnits     int64     `json:"offensiveWallUnits,omitempty"`
	OffensiveUnitThreshold int64     `json:"offensiveUnitThreshold,omitempty"`
	TriggeredAt            time.Time `json:"triggeredAt,omitempty"`
	GateOpenUntil          time.Time `json:"gateOpenUntil,omitempty"`
	Reason                 string    `json:"reason,omitempty"`
}

type KhanCooldownMSDState struct {
	WireKey        string    `json:"wireKey"`
	Minutes        int       `json:"minutes"`
	CooldownBefore int       `json:"cooldownBefore"`
	CooldownAfter  int       `json:"cooldownAfter"`
	AppliedAt      time.Time `json:"appliedAt"`
}

type KhanCooldownReportState struct {
	ReportID           int64                  `json:"reportId"`
	KingdomID          KingdomID              `json:"kingdomId"`
	X                  int                    `json:"x"`
	Y                  int                    `json:"y"`
	LandedAt           time.Time              `json:"landedAt"`
	CooldownRemaining  int                    `json:"cooldownRemaining,omitempty"`
	CooldownObservedAt time.Time              `json:"cooldownObservedAt,omitempty"`
	MSDs               []KhanCooldownMSDState `json:"msds,omitempty"`
	ResolvedAt         time.Time              `json:"resolvedAt,omitempty"`
}

type KhanState struct {
	RunID                     string                            `json:"runId,omitempty"`
	EventEndsAt               time.Time                         `json:"eventEndsAt,omitempty"`
	SourceCastleID            CastleID                          `json:"sourceCastleId,omitempty"`
	MainCastleID              CastleID                          `json:"mainCastleId,omitempty"`
	KingdomID                 KingdomID                         `json:"kingdomId,omitempty"`
	TargetX                   int                               `json:"targetX,omitempty"`
	TargetY                   int                               `json:"targetY,omitempty"`
	RageCampID                int64                             `json:"rageCampId,omitempty"`
	PlayerRage                int64                             `json:"playerRage,omitempty"`
	PlayerRageCap             int64                             `json:"playerRageCap,omitempty"`
	PlayerTotalRage           int64                             `json:"playerTotalRage,omitempty"`
	RageObservedAt            time.Time                         `json:"rageObservedAt,omitempty"`
	AttacksLaunched           int                               `json:"attacksLaunched"`
	VictoriesConfirmed        int                               `json:"victoriesConfirmed"`
	CooldownsSkipped          int                               `json:"cooldownsSkipped"`
	Launches                  []KhanLaunchState                 `json:"launches"`
	Taunts                    map[MovementID]KhanTauntState     `json:"taunts"`
	ResolvedTaunts            []KhanTauntState                  `json:"resolvedTaunts,omitempty"`
	TauntsTriggered           int                               `json:"tauntsTriggered"`
	TauntsObserved            int                               `json:"tauntsObserved"`
	TauntsResolved            int                               `json:"tauntsResolved"`
	TauntCounterVersion       int                               `json:"tauntCounterVersion"`
	LastTauntTriggeredAt      time.Time                         `json:"lastTauntTriggeredAt,omitempty"`
	LastTauntTriggeredRage    int64                             `json:"lastTauntTriggeredRage,omitempty"`
	LastTauntResolvedAt       time.Time                         `json:"lastTauntResolvedAt,omitempty"`
	LastReportID              int64                             `json:"lastReportId,omitempty"`
	LastAttackLaunchedAt      time.Time                         `json:"lastAttackLaunchedAt,omitempty"`
	LastCooldownSkippedAt     time.Time                         `json:"lastCooldownSkippedAt,omitempty"`
	LastDefenseToolPurchaseAt time.Time                         `json:"lastDefenseToolPurchaseAt,omitempty"`
	SafetyError               string                            `json:"safetyError,omitempty"`
	Protection                KhanProtectionState               `json:"protection"`
	CooldownReports           map[int64]KhanCooldownReportState `json:"cooldownReports"`
	CooldownReportVersion     int                               `json:"cooldownReportVersion"`
}

const KhanTauntCounterVersion = 1
const KhanCooldownReportVersion = 1

// AttackDialogState is the current pre-attack context returned by ADI. Its
// active effects are authoritative for the selected castle while the dialog
// remains current; a planned attack can therefore include temporary effects
// that are not represented by a building or inventory record.
type AttackDialogState struct {
	SourceCastleID CastleID             `json:"sourceCastleId,omitempty"`
	KingdomID      KingdomID            `json:"kingdomId,omitempty"`
	Target         AttackDialogTarget   `json:"target"`
	ActiveEffects  []AttackDialogEffect `json:"activeEffects"`
	ObservedAt     time.Time            `json:"observedAt,omitempty"`
}

type AttackDialogTarget struct {
	TypeID                     int      `json:"typeId,omitempty"`
	X                          int      `json:"x,omitempty"`
	Y                          int      `json:"y,omitempty"`
	ObjectID                   int64    `json:"objectId,omitempty"`
	OwnerID                    PlayerID `json:"ownerId,omitempty"`
	TowerVictoryCount          int64    `json:"towerVictoryCount,omitempty"`
	TowerCooldownRemaining     int      `json:"towerCooldownRemaining,omitempty"`
	EventCampID                int64    `json:"eventCampId,omitempty"`
	EventCampVictoryCount      int64    `json:"eventCampVictoryCount,omitempty"`
	EventCampCooldownRemaining int      `json:"eventCampCooldownRemaining,omitempty"`
	StormIsleID                int64    `json:"stormIsleId,omitempty"`
	StormVictoryCount          int64    `json:"stormVictoryCount,omitempty"`
	StormCooldownRemaining     int      `json:"stormCooldownRemaining,omitempty"`
}

type AttackDialogEffect struct {
	EffectID int64     `json:"effectId"`
	Values   []float64 `json:"values"`
	Source   string    `json:"source,omitempty"`
}

// AttackPreset is one of the game's saved six-lane attack formations. Units
// and tools each use left, front, and right lane indexes in that order.
type AttackPreset struct {
	Slot  int                    `json:"slot"`
	Name  string                 `json:"name,omitempty"`
	Units [3][]AttackPresetStack `json:"units"`
	Tools [3][]AttackPresetStack `json:"tools"`
}

type AttackPresetStack struct {
	DefinitionID int64 `json:"definitionId"`
	Amount       int64 `json:"amount"`
}

type ProtocolObservation struct {
	Opcode                        string    `json:"opcode"`
	Count                         uint64    `json:"count"`
	InboundCount                  uint64    `json:"inboundCount"`
	OutboundCount                 uint64    `json:"outboundCount"`
	LastDirection                 string    `json:"lastDirection"`
	LastCode                      *int      `json:"lastCode,omitempty"`
	LastError                     string    `json:"lastError,omitempty"`
	LastSeenAt                    time.Time `json:"lastSeenAt"`
	LastRevision                  uint64    `json:"lastRevision"`
	LastSuccessfulInboundAt       time.Time `json:"lastSuccessfulInboundAt,omitempty"`
	LastSuccessfulInboundRevision uint64    `json:"lastSuccessfulInboundRevision,omitempty"`
}

func (observation ProtocolObservation) SuccessfulInboundAt() time.Time {
	if !observation.LastSuccessfulInboundAt.IsZero() {
		return observation.LastSuccessfulInboundAt
	}
	if observation.LastDirection == "inbound" && observation.LastCode != nil && *observation.LastCode == 0 &&
		observation.LastError == "" {
		return observation.LastSeenAt
	}
	return time.Time{}
}

type ReportNotice struct {
	MessageID     int64     `json:"messageId"`
	TypeID        int       `json:"typeId"`
	BattleKey     string    `json:"battleKey,omitempty"`
	ReportID      int64     `json:"reportId,omitempty"`
	AgeSec        int64     `json:"ageSec,omitempty"`
	Status        string    `json:"status"`
	OwnedByPlayer bool      `json:"ownedByPlayer,omitempty"`
	ObservedAt    time.Time `json:"observedAt"`
}

type SpyReportCapture struct {
	MessageID  int64           `json:"messageId"`
	Payload    json.RawMessage `json:"payload"`
	CapturedAt time.Time       `json:"capturedAt"`
}

type BattleReportCapture struct {
	MessageID             int64             `json:"messageId"`
	ReportID              int64             `json:"reportId,omitempty"`
	BattleKey             string            `json:"battleKey,omitempty"`
	AutomationFeature     AttackFeatureID   `json:"automationFeature,omitempty"`
	MovementID            MovementID        `json:"movementId,omitempty"`
	EventID               int64             `json:"eventId,omitempty"`
	EventActivity         EventActivityKind `json:"eventActivity,omitempty"`
	EventOccurrenceEndsAt time.Time         `json:"eventOccurrenceEndsAt,omitempty"`
	ToolsUsed             int64             `json:"toolsUsed,omitempty"`
	Summary               json.RawMessage   `json:"summary,omitempty"`
	Waves                 json.RawMessage   `json:"waves,omitempty"`
	Details               json.RawMessage   `json:"details,omitempty"`
	OccurredAt            time.Time         `json:"occurredAt,omitempty"`
	CapturedAt            time.Time         `json:"capturedAt"`
}

type ReportState struct {
	Notices            map[int64]ReportNotice        `json:"notices"`
	SpyCaptures        map[int64]SpyReportCapture    `json:"spyCaptures"`
	BattleCaptures     map[int64]BattleReportCapture `json:"battleCaptures"`
	ActiveBattleReport int64                         `json:"activeBattleReport,omitempty"`
}

type CommandContextState struct {
	ProductionSessionKey int        `json:"productionSessionKey,omitempty"`
	ProductionObservedAt *time.Time `json:"productionObservedAt,omitempty"`
}

type DailyAttackState struct {
	Count           int64     `json:"count"`
	ServerThreshold int64     `json:"serverThreshold"`
	GrowthRate      float64   `json:"growthRate"`
	ObservedAt      time.Time `json:"observedAt,omitempty"`
}

type AutomationState struct {
	ID              string             `json:"id"`
	Enabled         bool               `json:"enabled"`
	Status          string             `json:"status"`
	Detail          string             `json:"detail,omitempty"`
	NextCheckAt     *time.Time         `json:"nextCheckAt,omitempty"`
	LastRunAt       *time.Time         `json:"lastRunAt,omitempty"`
	LastOperationID string             `json:"lastOperationId,omitempty"`
	LastError       string             `json:"lastError,omitempty"`
	Metrics         map[string]float64 `json:"metrics,omitempty"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

type GameState struct {
	SchemaVersion        int                                     `json:"schemaVersion"`
	Revision             uint64                                  `json:"revision"`
	UpdatedAt            time.Time                               `json:"updatedAt"`
	CatalogVersion       string                                  `json:"catalogVersion,omitempty"`
	LanguageVersion      string                                  `json:"languageVersion,omitempty"`
	Session              SessionState                            `json:"session"`
	Account              AccountBindingState                     `json:"account"`
	Player               PlayerState                             `json:"player"`
	Castles              map[CastleID]CastleState                `json:"castles"`
	Commanders           map[CommanderID]CommanderState          `json:"commanders"`
	Generals             map[int64]GeneralState                  `json:"generals"`
	Castellans           map[CastellanID]CastellanState          `json:"castellans"`
	Movements            map[MovementID]MovementState            `json:"movements"`
	MovementSnapshot     MovementSnapshot                        `json:"movementSnapshot"`
	Stationing           map[string]StationingOperation          `json:"stationing"`
	Scheduled            map[string]ScheduledOperation           `json:"scheduled"`
	Rift                 RiftState                               `json:"rift"`
	Inventory            InventoryState                          `json:"inventory"`
	Subscriptions        map[int]SubscriptionState               `json:"subscriptions"`
	Market               MarketState                             `json:"market"`
	KingdomTransport     KingdomTransportState                   `json:"kingdomTransport"`
	Beri                 BeriState                               `json:"beri"`
	Alliance             AllianceState                           `json:"alliance"`
	Alliances            map[AllianceID]AllianceState            `json:"alliances"`
	AllianceHelpRequests AllianceHelpRequestState                `json:"allianceHelpRequests"`
	Map                  map[KingdomID]map[string]MapObservation `json:"-"`
	TowerCooldowns       map[string]TowerCooldownState           `json:"towerCooldowns"`
	TowerQueue           TowerQueueState                         `json:"towerQueue"`
	Invasion             InvasionState                           `json:"invasion"`
	Storm                StormState                              `json:"storm"`
	NomadCamps           NomadCampState                          `json:"nomadCamps"`
	Advisor              AdvisorState                            `json:"advisor"`
	Khan                 KhanState                               `json:"khan"`
	DailyAttacks         DailyAttackState                        `json:"dailyAttacks"`
	AttackDialog         AttackDialogState                       `json:"attackDialog"`
	AttackPresets        []AttackPreset                          `json:"attackPresets"`
	AttackAnalytics      AttackAnalyticsState                    `json:"attackAnalytics"`
	EventScores          EventScoreState                         `json:"eventScores"`
	CommandContext       CommandContextState                     `json:"commandContext"`
	Automations          map[string]AutomationState              `json:"automations"`
	Reports              ReportState                             `json:"reports"`
	Observations         map[string]ProtocolObservation          `json:"observations"`

	sharedMap                   *worldMapGeneration
	mapOverlay                  *accountMapGeneration
	worldSharing                bool
	pendingMapChanges           map[string]MapChange
	replaceMap                  bool
	mapMutationCOW              bool
	mutableMapRegions           map[KingdomID]*mutableAccountMapRegion
	movementRecords             *movementGeneration
	movementMutationCOW         bool
	mutableMovementShards       [4]uint64
	pendingMovementChanges      map[MovementID]struct{}
	replaceMovements            bool
	castleMutationCOW           bool
	mutableCastles              map[CastleID]CastleMutationPart
	pendingCastleChanges        map[CastleID]CastleMutationPart
	replaceCastles              bool
	inventoryMutationCOW        bool
	mutableInventoryParts       inventoryMutationPart
	pendingEquipmentChanges     map[EquipmentInstanceID]struct{}
	replaceInventoryEquipment   bool
	pendingGemChanges           map[GemInstanceID]struct{}
	replaceInventoryGems        bool
	pendingInventoryItemChanges map[string]struct{}
	replaceInventoryItems       bool
	stormTargets                *stormTargetGeneration
	stormMutationCOW            bool
	mutableStormParts           stormMutationPart
	mutableStormTargetShards    [4]uint64
	pendingStormTargetChanges   map[string]struct{}
	replaceStormTargets         bool
	towerCooldowns              *towerCooldownGeneration
	towerCooldownMutationCOW    bool
	mutableTowerCooldownShards  [4]uint64
	pendingTowerCooldownChanges map[string]struct{}
	replaceTowerCooldowns       bool
	towerQueueMutationCOW       bool
	mutableTowerQueueEntries    map[CastleID]struct{}
	pendingTowerQueueCastles    map[CastleID]struct{}
	replaceTowerQueue           bool
	reportMutationCOW           bool
	reportRecords               *reportGeneration
	mutableReportShards         [4]uint64
	pendingReportMessages       map[int64]struct{}
	replaceReports              bool
	eventScoreMutationCOW       bool
	eventScoreRecords           *eventScoreGeneration
	mutableEventScoreShards     [4]uint64
	pendingEventScoreIDs        map[int64]struct{}
	eventScoreMetadataDirty     bool
	eventScoreShopDirty         bool
	replaceEventScores          bool
	attackAnalyticsMutationCOW  bool
	mutableAttackAnalyticsParts attackAnalyticsMutationPart
	mutationWrites              ComponentSet
}

func NewGameState() GameState {
	now := time.Now().UTC()
	return GameState{
		SchemaVersion: SchemaVersion,
		UpdatedAt:     now,
		Session:       SessionState{Status: "stopped", Namespace: "EmpireEx_21", ChangedAt: now},
		Player: PlayerState{
			Resources: map[ResourceID]float64{}, Currencies: map[CurrencyID]float64{},
			Achievements: AchievementState{Completed: map[int64]bool{}, Progress: map[int64][]int64{}},
			LegendSkills: LegendSkillState{ActiveIDs: []int64{}, SceatSkillIDs: []int64{}, SceatActivations: []SceatSkillActivation{}},
		},
		Castles:    map[CastleID]CastleState{},
		Commanders: map[CommanderID]CommanderState{},
		Generals:   map[int64]GeneralState{},
		Castellans: map[CastellanID]CastellanState{},
		Movements:  map[MovementID]MovementState{},
		Stationing: map[string]StationingOperation{},
		Scheduled:  map[string]ScheduledOperation{},
		Rift:       RiftState{Launches: map[string]RiftLaunch{}, DeletedLaunchIDs: map[string]int64{}},
		Inventory: InventoryState{
			ConstructionItems:          map[ConstructionItemID]int64{},
			ConstructionOffers:         map[PackageID]int64{},
			ConstructionOffersByCastle: map[CastleID]ConstructionOfferSnapshot{},
			Equipment:                  map[EquipmentInstanceID]EquipmentInstance{},
			Gems:                       map[GemInstanceID]GemInstance{},
			GemStacks:                  map[GemID]int64{},
			Items:                      map[string]map[int64]int64{},
		},
		Subscriptions: map[int]SubscriptionState{},
		Market: MarketState{
			Castles: map[CastleID]MarketCastleState{}, Boosters: map[int]MarketBoosterState{},
		},
		KingdomTransport: KingdomTransportState{
			Unlocks: map[KingdomID]KingdomTransportUnlock{}, Pending: []KingdomResourceTransport{},
			PendingUnits: []KingdomUnitTransport{}, ResourceWorkflows: map[KingdomID]KingdomResourceTransportWorkflow{},
		},
		Beri:      BeriState{TroopsByUnit: map[UnitID]int64{}},
		Alliance:  AllianceState{Members: []AllianceMember{}, Holdings: []AllianceHolding{}},
		Alliances: map[AllianceID]AllianceState{},
		AllianceHelpRequests: AllianceHelpRequestState{
			HospitalProductionIDs: []int64{}, RecruitmentCastleIDs: []CastleID{}, PendingOtherListIDs: []int64{},
		},
		Map:            map[KingdomID]map[string]MapObservation{},
		TowerCooldowns: map[string]TowerCooldownState{},
		TowerQueue: TowerQueueState{
			EntriesByCastle: map[CastleID][]TowerQueueEntry{}, LastScannedAt: map[CastleID]time.Time{},
			LastAttemptedAt:           map[CastleID]time.Time{},
			ConfirmedLaunchesByCastle: map[CastleID]int64{},
			CursorVersion:             TowerQueueCursorVersion,
			CapacityByCastle:          map[CastleID]TowerCapacityObservation{},
		},
		Invasion: InvasionState{
			LastScannedAt: map[CastleID]time.Time{}, FortifiedTargets: map[string]string{}, FortifyCurrencies: []string{},
		},
		Storm: StormState{
			LastScannedAt: map[CastleID]time.Time{},
			Map:           StormMapState{Targets: map[string]MapObservation{}},
			IslandReturns: map[string]StormIslandReturnState{},
		},
		NomadCamps: NomadCampState{
			LastScannedAt: map[CastleID]time.Time{}, Cooldowns: map[string]NomadCampCooldownState{},
		},
		Advisor: AdvisorState{Summary: AdvisorSummaryState{Gains: map[string]int64{}, Costs: map[string]int64{}}},
		Khan: KhanState{
			Launches: []KhanLaunchState{}, Taunts: map[MovementID]KhanTauntState{},
			TauntCounterVersion:   KhanTauntCounterVersion,
			CooldownReports:       map[int64]KhanCooldownReportState{},
			CooldownReportVersion: KhanCooldownReportVersion,
		},
		AttackPresets: []AttackPreset{},
		AttackAnalytics: AttackAnalyticsState{
			LaunchIDs: []MovementID{}, PendingAttacks: []AttackFeatureLaunch{}, RecentAutoStormLaunches: []AttackFeatureLaunch{},
		},
		EventScores: EventScoreState{
			ByEvent: map[int64]ScalableEventScore{}, ShopByPackage: map[PackageID]EventShopRoute{},
			ActivityByEvent: map[int64]EventActivityState{}, RankingByEvent: map[int64]EventRankingState{},
			Inventory: EventInventoryState{ActiveByEvent: map[int64]EventAvailability{}},
		},
		Automations: map[string]AutomationState{},
		Reports: ReportState{
			Notices: map[int64]ReportNotice{}, SpyCaptures: map[int64]SpyReportCapture{},
			BattleCaptures: map[int64]BattleReportCapture{},
		},
		Observations: map[string]ProtocolObservation{},
	}
}
