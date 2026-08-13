package Reports

import (
	"encoding/json"
	"time"

	"CitadelDesktop/Server/State"
)

const (
	BattleResearchConfigurationSection = "research.battlePredictionBeta"
	BattleResearchConsentVersion       = 1
	BattleResearchModelVersion         = "catalog-effect-combat-v2"
)

type BattleResearchConfiguration struct {
	Enabled        bool `json:"enabled"`
	ConsentVersion int  `json:"consentVersion"`
	SpyCount       int  `json:"spyCount,omitempty"`
}

func (configuration BattleResearchConfiguration) Active() bool {
	return configuration.Enabled && configuration.ConsentVersion == BattleResearchConsentVersion
}

type BattleResearchFormation struct {
	CapturedAt     time.Time         `json:"capturedAt"`
	SourceX        int               `json:"sourceX"`
	SourceY        int               `json:"sourceY"`
	TargetX        int               `json:"targetX"`
	TargetY        int               `json:"targetY"`
	KingdomID      State.KingdomID   `json:"kingdomID"`
	CommanderID    State.CommanderID `json:"commanderID"`
	Raw            json.RawMessage   `json:"raw"`
	Waves          []ResearchWave    `json:"waves"`
	SupportTroops  []ResearchPair    `json:"supportTroops,omitempty"`
	SupportToolIDs []int64           `json:"supportToolIDs,omitempty"`
}

type ResearchPair struct {
	WodID  int64 `json:"wodID"`
	Amount int64 `json:"amount"`
}

type ResearchLane struct {
	Units []ResearchPair `json:"units"`
	Tools []ResearchPair `json:"tools"`
}

type ResearchWave struct {
	Index  int          `json:"index"`
	Left   ResearchLane `json:"left"`
	Center ResearchLane `json:"center"`
	Right  ResearchLane `json:"right"`
}

type BattleResearchAttackerContext struct {
	CapturedAt     time.Time                 `json:"capturedAt"`
	CatalogVersion string                    `json:"catalogVersion,omitempty"`
	Commander      State.CommanderState      `json:"commander"`
	General        *State.GeneralState       `json:"general,omitempty"`
	Equipment      []State.EquipmentInstance `json:"equipment,omitempty"`
	Gems           []State.GemInstance       `json:"gems,omitempty"`
	LegendSkills   State.LegendSkillState    `json:"legendSkills"`
	AttackDialog   *State.AttackDialogState  `json:"attackDialog,omitempty"`
}

type BattleResearchSpySnapshot struct {
	Purpose    string                 `json:"purpose"`
	CapturedAt time.Time              `json:"capturedAt"`
	Report     SpyReport              `json:"report"`
	RawCapture State.SpyReportCapture `json:"rawCapture"`
}

type BattleResearchOutcome struct {
	CapturedAt time.Time                 `json:"capturedAt"`
	Report     BattleReport              `json:"report"`
	RawCapture State.BattleReportCapture `json:"rawCapture"`
}

type BattlePrediction struct {
	ModelVersion              string                 `json:"modelVersion"`
	GeneratedAt               time.Time              `json:"generatedAt"`
	PredictedResult           string                 `json:"predictedResult"`
	AttackWinProbability      float64                `json:"attackWinProbability"`
	Confidence                string                 `json:"confidence"`
	AttackerSent              int64                  `json:"attackerSent"`
	DefenderObserved          int64                  `json:"defenderObserved"`
	ExpectedAttackerLost      int64                  `json:"expectedAttackerLost"`
	ExpectedDefenderLost      int64                  `json:"expectedDefenderLost"`
	ExpectedAttackerSurvivors int64                  `json:"expectedAttackerSurvivors"`
	ExpectedDefenderSurvivors int64                  `json:"expectedDefenderSurvivors"`
	UnitStatCoverage          float64                `json:"unitStatCoverage"`
	AttackerMeleeBonus        float64                `json:"attackerMeleeBonusPercent,omitempty"`
	AttackerRangeBonus        float64                `json:"attackerRangeBonusPercent,omitempty"`
	WallReduction             float64                `json:"wallReductionPercent,omitempty"`
	GateReduction             float64                `json:"gateReductionPercent,omitempty"`
	MoatReduction             float64                `json:"moatReductionPercent,omitempty"`
	Waves                     []BattleWavePrediction `json:"waves"`
	Courtyard                 BattlePhasePrediction  `json:"courtyard"`
	Considered                []string               `json:"considered"`
	RecordedNotModeled        []string               `json:"recordedNotModeled"`
	Assumptions               []string               `json:"assumptions"`
}

type BattleWavePrediction struct {
	Wave   int                   `json:"wave"`
	Left   BattlePhasePrediction `json:"left"`
	Center BattlePhasePrediction `json:"center"`
	Right  BattlePhasePrediction `json:"right"`
}

type BattlePhasePrediction struct {
	Winner            string  `json:"winner"`
	AttackerStarted   int64   `json:"attackerStarted"`
	DefenderStarted   int64   `json:"defenderStarted"`
	AttackerPower     float64 `json:"attackerPower"`
	DefenderPower     float64 `json:"defenderPower"`
	AttackerLost      int64   `json:"attackerLost"`
	DefenderLost      int64   `json:"defenderLost"`
	AttackerSurvivors int64   `json:"attackerSurvivors"`
	DefenderSurvivors int64   `json:"defenderSurvivors"`
}

type BattleResearchTrial struct {
	Version            int                           `json:"version"`
	ID                 string                        `json:"id"`
	Phase              string                        `json:"phase"`
	AccountUID         int64                         `json:"accountUID,omitempty"`
	WorldID            string                        `json:"worldID,omitempty"`
	PlayerID           State.PlayerID                `json:"playerID"`
	CreatedAt          time.Time                     `json:"createdAt"`
	UpdatedAt          time.Time                     `json:"updatedAt"`
	Formation          BattleResearchFormation       `json:"formation"`
	AttackerContext    BattleResearchAttackerContext `json:"attackerContext"`
	Movement           *State.MovementState          `json:"movement,omitempty"`
	PreSpyRequestedAt  time.Time                     `json:"preSpyRequestedAt,omitempty"`
	PreSpyAttempts     int                           `json:"preSpyAttempts,omitempty"`
	PreSpy             *BattleResearchSpySnapshot    `json:"preSpy,omitempty"`
	Prediction         *BattlePrediction             `json:"prediction,omitempty"`
	Battle             *BattleResearchOutcome        `json:"battle,omitempty"`
	PostSpyRequestedAt time.Time                     `json:"postSpyRequestedAt,omitempty"`
	PostSpyAttempts    int                           `json:"postSpyAttempts,omitempty"`
	PostSpy            *BattleResearchSpySnapshot    `json:"postSpy,omitempty"`
	CompletedAt        time.Time                     `json:"completedAt,omitempty"`
	UploadState        string                        `json:"uploadState"`
	UploadedAt         time.Time                     `json:"uploadedAt,omitempty"`
	NextActionAt       time.Time                     `json:"nextActionAt,omitempty"`
	LastError          string                        `json:"lastError,omitempty"`
}

type BattleResearchTrialSummary struct {
	ID                 string            `json:"id"`
	Phase              string            `json:"phase"`
	MovementID         int64             `json:"movementID,omitempty"`
	TargetX            int               `json:"targetX"`
	TargetY            int               `json:"targetY"`
	KingdomID          State.KingdomID   `json:"kingdomID"`
	ArrivesAt          *time.Time        `json:"arrivesAt,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	Prediction         *BattlePrediction `json:"prediction,omitempty"`
	ActualResult       string            `json:"actualResult,omitempty"`
	ActualAttackerLost *int64            `json:"actualAttackerLost,omitempty"`
	ActualDefenderLost *int64            `json:"actualDefenderLost,omitempty"`
	UploadState        string            `json:"uploadState"`
	LastError          string            `json:"lastError,omitempty"`
}

type BattleResearchCalculatorInfo struct {
	ModelVersion string   `json:"modelVersion"`
	Maturity     string   `json:"maturity"`
	Description  string   `json:"description"`
	Considered   []string `json:"considered"`
	Limitations  []string `json:"limitations"`
}

type BattleResearchStatus struct {
	Beta                   bool                         `json:"beta"`
	Enabled                bool                         `json:"enabled"`
	ConsentVersion         int                          `json:"consentVersion"`
	RequiredConsentVersion int                          `json:"requiredConsentVersion"`
	State                  string                       `json:"state"`
	ActiveTrials           int                          `json:"activeTrials"`
	CompletedTrials        int                          `json:"completedTrials"`
	PendingUploads         int                          `json:"pendingUploads"`
	LastMovementPollAt     time.Time                    `json:"lastMovementPollAt,omitempty"`
	LastError              string                       `json:"lastError,omitempty"`
	Calculator             BattleResearchCalculatorInfo `json:"calculator"`
	Trials                 []BattleResearchTrialSummary `json:"trials"`
}

func researchCalculatorInfo() BattleResearchCalculatorInfo {
	return BattleResearchCalculatorInfo{
		ModelVersion: BattleResearchModelVersion,
		Maturity:     "experimental-low-confidence",
		Description:  "A catalog-backed deterministic calculator calibrated on complete battle packets; it is a research estimate, not a guaranteed battle result.",
		Considered: []string{
			"exact per-wave and per-lane attacker formation",
			"official melee and ranged unit combat values",
			"observed defender deployment from the pre-battle spy report",
			"capped commander, castellan, gem, legend-skill, and active combat effects",
			"attacker lane and courtyard support tools, including fixed pre-yard losses",
			"defender-side melee, ranged, global, lane, courtyard, wall, gate, and moat effects",
			"official wall, gate, and moat levels plus one-, two-, or three-section courtyard control",
		},
		Limitations: []string{
			"espionage does not reveal defender wall-tool placement, so those tool bonuses remain unknown before combat",
			"general abilities with phase-dependent triggers cannot be resolved exactly before combat",
			"some nonstandard target types expose fortification levels without a matching catalog base-bonus row",
			"defender changes after the pre-battle spy snapshot cannot be predicted",
			"casualty allocation is calibrated but remains an approximation until forward-tested coverage grows",
		},
	}
}
