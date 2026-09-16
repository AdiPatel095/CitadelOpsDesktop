package Equipment

import "CitadelDesktop/Server/State"

type Priority struct {
	EffectID int64 `json:"effectId"`
	Tier     int   `json:"tier"`
	Position int   `json:"position"`
}

type OptimizeRequest struct {
	LeaderKind        string     `json:"leaderKind"`
	LeaderID          int64      `json:"leaderId"`
	CombatMode        string     `json:"combatMode"`
	TargetAreaTypeIDs []int64    `json:"targetAreaTypeIds,omitempty"`
	Priorities        []Priority `json:"priorities"`
	ResultCount       int        `json:"resultCount,omitempty"`
}

type EffectTotal struct {
	SemanticKey  string   `json:"semanticKey"`
	DefinitionID int64    `json:"definitionId"`
	ArgumentID   *int64   `json:"argumentId,omitempty"`
	Value        float64  `json:"value"`
	RawValue     float64  `json:"rawValue"`
	Unit         string   `json:"unit"`
	Precision    int      `json:"precision"`
	Categorical  bool     `json:"categorical,omitempty"`
	CapID        int64    `json:"capId,omitempty"`
	Cap          *float64 `json:"cap,omitempty"`
	Capped       bool     `json:"capped"`
}

type Loadout struct {
	Equipment           map[string]State.EquipmentInstanceID `json:"equipment"`
	Gems                map[string]State.GemInstanceID       `json:"gems"`
	Effects             []EffectTotal                        `json:"effects"`
	Score               float64                              `json:"score"`
	ExtractionCost      ExtractionQuote                      `json:"extractionCost"`
	Reason              string                               `json:"reason,omitempty"`
	Useful              bool                                 `json:"useful"`
	priorityValues      map[int64]float64
	semanticDefinitions map[string]map[int64]struct{}
}

type CandidateCounts struct {
	EquipmentBySlot map[string]int `json:"equipmentBySlot"`
	Gems            int            `json:"gems"`
}

type OptimizeResponse struct {
	LeaderKind          string          `json:"leaderKind"`
	LeaderID            int64           `json:"leaderId"`
	StateRevision       uint64          `json:"stateRevision"`
	SnapshotFingerprint string          `json:"snapshotFingerprint"`
	Current             Loadout         `json:"current"`
	Proposed            Loadout         `json:"proposed"`
	Alternatives        []Loadout       `json:"alternatives"`
	Candidates          CandidateCounts `json:"candidates"`
	NoUsefulChange      bool            `json:"noUsefulChange,omitempty"`
}
