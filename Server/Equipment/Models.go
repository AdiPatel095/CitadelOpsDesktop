package Equipment

import "CitadelDesktop/Server/State"

type Priority struct {
	EffectID int64 `json:"effectId"`
	Tier     int   `json:"tier"`
	Position int   `json:"position"`
}

type OptimizeRequest struct {
	LeaderKind  string     `json:"leaderKind"`
	LeaderID    int64      `json:"leaderId"`
	CombatMode  string     `json:"combatMode"`
	Priorities  []Priority `json:"priorities"`
	ResultCount int        `json:"resultCount,omitempty"`
}

type EffectTotal struct {
	DefinitionID int64    `json:"definitionId"`
	Value        float64  `json:"value"`
	CapID        int64    `json:"capId,omitempty"`
	Cap          *float64 `json:"cap,omitempty"`
	Capped       bool     `json:"capped"`
}

type Loadout struct {
	Equipment      map[string]State.EquipmentInstanceID `json:"equipment"`
	Gems           map[string]State.GemInstanceID       `json:"gems"`
	Effects        []EffectTotal                        `json:"effects"`
	Score          float64                              `json:"score"`
	ExtractionCost ExtractionQuote                      `json:"extractionCost"`
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
}
