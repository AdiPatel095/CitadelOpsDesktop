package AttackCapacity

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	flankEffectTypeID                       = 28
	frontEffectTypeID                       = 34
	waveEffectTypeID                        = 156
	supportUnitsEffectTypeID                = 179
	supportBoostEffectTypeID                = 180
	baseAttackWaves                         = 4
	basePvEFlankToolCapacity          int64 = 30
	basePvEFrontToolCapacity          int64 = 40
	baseLegendaryPvPFlankToolCapacity int64 = 40
	baseLegendaryPvPFrontToolCapacity int64 = 50
	maximumLegendToolBonus            int64 = 10
)

type Lane string

const (
	LaneFlank Lane = "flank"
	LaneFront Lane = "front"
)

type TargetType string

const (
	TargetTypeBerimondTower TargetType = "berimond_tower"
	BerimondTowerMapTypeID             = 17
)

// LaneCapacity is the maximum troop count for one wave in each attack lane.
// The center lane is named Front to match the game's official effect type.
type LaneCapacity struct {
	Left  int64 `json:"left"`
	Front int64 `json:"front"`
	Right int64 `json:"right"`
}

type LaneBonus struct {
	Left  float64 `json:"left"`
	Front float64 `json:"front"`
	Right float64 `json:"right"`
}

// MapTarget identifies the target selected in the map or pre-attack dialog.
// Level or VictoryCount lets the resolver reproduce the game's target-specific
// base lane limits when BaseCapacity is not supplied explicitly.
type MapTarget struct {
	KingdomID    State.KingdomID `json:"kingdomId"`
	TypeID       int             `json:"typeId"`
	X            int             `json:"x"`
	Y            int             `json:"y"`
	ObjectID     int64           `json:"objectId,omitempty"`
	Level        int             `json:"level,omitempty"`
	VictoryCount int64           `json:"victoryCount,omitempty"`
}

// TargetContext may provide an explicit BaseCapacity for special targets. For
// normal targets, Level (or a dungeon map VictoryCount) is enough to infer it.
type TargetContext struct {
	ID           string     `json:"id,omitempty"`
	Map          *MapTarget `json:"map,omitempty"`
	TargetType   TargetType `json:"targetType,omitempty"`
	Level        int        `json:"level,omitempty"`
	CastleTypeID int        `json:"castleTypeId,omitempty"`
	// AreaTypeID is retained as a compatibility alias for older callers.
	// CastleTypeID is the authoritative official effect discriminator.
	AreaTypeID     int          `json:"areaTypeId"`
	PvP            bool         `json:"pvp"`
	LegendaryFight bool         `json:"legendaryFight,omitempty"`
	BaseCapacity   LaneCapacity `json:"baseCapacity"`
	levelDerived   bool
}

type SourceKind string

const (
	SourceBuilding         SourceKind = "building"
	SourceDecoration       SourceKind = "decoration"
	SourceConstructionItem SourceKind = "construction_item"
	SourceEquipment        SourceKind = "equipment"
	SourceGem              SourceKind = "gem"
	SourceEquipmentSet     SourceKind = "equipment_set"
	SourceAttackDialog     SourceKind = "attack_dialog"
	SourceAllianceBonus    SourceKind = "alliance_bonus"
	SourceTitle            SourceKind = "title"
	SourceResearch         SourceKind = "research"
	SourceGlobalEffect     SourceKind = "global_effect"
	SourceAccountEffect    SourceKind = "account_effect"
	SourceGeneralSkill     SourceKind = "general_skill"
	SourceLegendSkill      SourceKind = "legend_skill"
	SourceAdditional       SourceKind = "additional"
)

// EffectSource is used for account, event, or target effects that are not yet
// represented in GameState. DefinitionID should refer to the official effects
// catalog; WireID is accepted as a fallback for captured effect rows.
type EffectSource struct {
	Kind    SourceKind             `json:"kind"`
	ID      string                 `json:"id,omitempty"`
	Label   string                 `json:"label,omitempty"`
	Effects State.EquipmentEffects `json:"effects"`
}

type Request struct {
	// SourceCastleID may be omitted to use the currently focused castle.
	SourceCastleID State.CastleID    `json:"sourceCastleId,omitempty"`
	CommanderID    State.CommanderID `json:"commanderId"`
	Target         TargetContext     `json:"target"`
	// UseAttackDialogEffects replaces the state-derived castle and
	// construction-item effects with the active effects in the latest ADI
	// response. Commander equipment, gems, and set bonuses remain derived from
	// state because ADI is opened before a commander is chosen.
	UseAttackDialogEffects bool           `json:"useAttackDialogEffects,omitempty"`
	AdditionalSources      []EffectSource `json:"additionalSources,omitempty"`
	// EvaluatedAt makes temporary construction-item expiry deterministic. The
	// current time is used when callers omit it.
	EvaluatedAt time.Time `json:"evaluatedAt,omitempty"`
}

// Modifier is one official capacity effect that applies to the requested
// target. Percent is additive within its official cap group.
type Modifier struct {
	SourceKind  SourceKind `json:"sourceKind"`
	SourceID    string     `json:"sourceId,omitempty"`
	SourceLabel string     `json:"sourceLabel,omitempty"`
	EffectID    int64      `json:"effectId"`
	EffectName  string     `json:"effectName"`
	Lane        Lane       `json:"lane"`
	CapID       int64      `json:"capId"`
	Percent     float64    `json:"percent"`
}

// WaveModifier is one target-applicable effect that adds whole attack waves.
type WaveModifier struct {
	SourceKind  SourceKind `json:"sourceKind"`
	SourceID    string     `json:"sourceId,omitempty"`
	SourceLabel string     `json:"sourceLabel,omitempty"`
	EffectID    int64      `json:"effectId"`
	EffectName  string     `json:"effectName"`
	Waves       int        `json:"waves"`
}

type SupportModifierKind string

const (
	SupportModifierUnits SupportModifierKind = "units"
	SupportModifierBoost SupportModifierKind = "boost"
)

// SupportModifier is one target-applicable effect that supplies courtyard
// reinforcement units or boosts the assembled reinforcement capacity.
type SupportModifier struct {
	SourceKind  SourceKind          `json:"sourceKind"`
	SourceID    string              `json:"sourceId,omitempty"`
	SourceLabel string              `json:"sourceLabel,omitempty"`
	EffectID    int64               `json:"effectId"`
	EffectName  string              `json:"effectName"`
	Kind        SupportModifierKind `json:"kind"`
	CapID       int64               `json:"capId"`
	Amount      float64             `json:"amount"`
}

type SkippedEffect struct {
	SourceKind SourceKind `json:"sourceKind"`
	SourceID   string     `json:"sourceId,omitempty"`
	EffectID   int64      `json:"effectId"`
	EffectName string     `json:"effectName,omitempty"`
	Reason     string     `json:"reason"`
}

// CapGroup exposes the game's official maxTotalBonus calculation for each
// applicable lane and cap id. A cap id without a maxTotalBonus is uncapped.
type CapGroup struct {
	Lane           Lane    `json:"lane"`
	CapID          int64   `json:"capId"`
	RawPercent     float64 `json:"rawPercent"`
	AppliedPercent float64 `json:"appliedPercent"`
	MaximumPercent float64 `json:"maximumPercent,omitempty"`
	Capped         bool    `json:"capped"`
}

// SupportCapGroup exposes the official cap calculation separately for absolute
// support units and percentage support boosts.
type SupportCapGroup struct {
	Kind          SupportModifierKind `json:"kind"`
	CapID         int64               `json:"capId"`
	RawAmount     float64             `json:"rawAmount"`
	AppliedAmount float64             `json:"appliedAmount"`
	MaximumAmount float64             `json:"maximumAmount,omitempty"`
	Capped        bool                `json:"capped"`
}

// Context is deliberately transport-free. Future refresh intents and target
// parsers can build the input state separately, then use this as the stable,
// testable calculation boundary.
type Context struct {
	SourceCastleID   State.CastleID    `json:"sourceCastleId"`
	CommanderID      State.CommanderID `json:"commanderId"`
	Target           TargetContext     `json:"target"`
	BaseWaves        int               `json:"baseWaves"`
	LegendToolBonus  int64             `json:"legendToolBonus,omitempty"`
	WaveModifiers    []WaveModifier    `json:"waveModifiers,omitempty"`
	SupportModifiers []SupportModifier `json:"supportModifiers,omitempty"`
	Modifiers        []Modifier        `json:"modifiers"`
	Skipped          []SkippedEffect   `json:"skipped,omitempty"`
	CapLimits        map[int64]float64 `json:"-"`
}

type Result struct {
	SourceCastleID      State.CastleID    `json:"sourceCastleId"`
	CommanderID         State.CommanderID `json:"commanderId"`
	Target              TargetContext     `json:"target"`
	BaseWaves           int               `json:"baseWaves"`
	AdditionalWaves     int               `json:"additionalWaves"`
	MaximumWaves        int               `json:"maximumWaves"`
	WaveModifiers       []WaveModifier    `json:"waveModifiers,omitempty"`
	BaseCapacity        LaneCapacity      `json:"baseCapacity"`
	BonusPercent        LaneBonus         `json:"bonusPercent"`
	Capacity            LaneCapacity      `json:"capacity"`
	BaseToolCapacity    LaneCapacity      `json:"baseToolCapacity"`
	LegendToolBonus     int64             `json:"legendToolBonus"`
	ToolCapacity        LaneCapacity      `json:"toolCapacity"`
	SupportUnitAmount   float64           `json:"supportUnitAmount"`
	SupportBoostPercent float64           `json:"supportBoostPercent"`
	SupportCapacity     int64             `json:"supportCapacity"`
	SupportModifiers    []SupportModifier `json:"supportModifiers,omitempty"`
	SupportCapGroups    []SupportCapGroup `json:"supportCapGroups,omitempty"`
	Modifiers           []Modifier        `json:"modifiers"`
	CapGroups           []CapGroup        `json:"capGroups"`
	Skipped             []SkippedEffect   `json:"skipped,omitempty"`
}

type Resolver struct{}

// Resolve collects known castle, construction-item, commander-equipment, gem,
// equipment-set, and skill effects before resolving attack waves, lane limits,
// and courtyard-support capacity.
func (Resolver) Resolve(gameState State.GameState, gameData *GameData.Store, request Request) (Result, error) {
	context, err := (Resolver{}).BuildContext(gameState, gameData, request)
	if err != nil {
		return Result{}, err
	}
	return (Resolver{}).ResolveContext(context)
}

// BuildContext collects only attack-limit effects whose official metadata and
// PvP/PvE or area restrictions match the requested target.
func (Resolver) BuildContext(gameState State.GameState, gameData *GameData.Store, request Request) (Context, error) {
	if gameData == nil {
		return Context{}, fmt.Errorf("official game data is unavailable")
	}
	if request.CommanderID < 0 {
		return Context{}, fmt.Errorf("commander id cannot be negative")
	}
	sourceCastleID, castle, err := resolvedSourceCastle(gameState, request.SourceCastleID)
	if err != nil {
		return Context{}, err
	}
	target := request.Target
	if request.UseAttackDialogEffects {
		if gameState.AttackDialog.SourceCastleID <= 0 {
			return Context{}, fmt.Errorf("the pre-attack dialog has not been observed")
		}
		if gameState.AttackDialog.SourceCastleID != sourceCastleID {
			return Context{}, fmt.Errorf("the pre-attack dialog belongs to castle %d, not source castle %d", gameState.AttackDialog.SourceCastleID, sourceCastleID)
		}
		if err := validateAttackDialogTarget(target, gameState.AttackDialog); err != nil {
			return Context{}, err
		}
		target = targetWithAttackDialog(target, gameState.AttackDialog)
	}
	target = targetWithSemanticType(target)
	target = targetWithCastleType(target)
	target = targetWithInferredBaseCapacity(target)
	if err := validateTarget(target); err != nil {
		return Context{}, err
	}
	commander, exists := gameState.Commanders[request.CommanderID]
	if !exists {
		return Context{}, fmt.Errorf("commander %d is not in the current player state", request.CommanderID)
	}
	effects, err := gameData.Catalog("effects")
	if err != nil {
		return Context{}, fmt.Errorf("official effect catalog is unavailable: %w", err)
	}

	context := Context{
		SourceCastleID: sourceCastleID,
		CommanderID:    request.CommanderID,
		Target:         target,
		BaseWaves:      baseAttackWaves,
		CapLimits:      loadCapLimits(gameData),
	}
	evaluatedAt := request.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}
	builder := contextBuilder{
		gameData: gameData, effects: effects, target: target, context: &context, evaluatedAt: evaluatedAt,
	}
	if request.UseAttackDialogEffects {
		builder.addAttackDialogEffects(gameState.AttackDialog)
	} else {
		if err := builder.addCastleBuildings(castle); err != nil {
			return Context{}, err
		}
		if err := builder.addConstructionItems(castle); err != nil {
			return Context{}, err
		}
	}
	setCounts, err := builder.addCommanderEquipment(gameState, commander)
	if err != nil {
		return Context{}, err
	}
	if err := builder.addCommanderGems(gameState, commander, setCounts); err != nil {
		return Context{}, err
	}
	if err := builder.addEquipmentSetBonuses(setCounts); err != nil {
		return Context{}, err
	}
	if err := builder.addGeneralSkills(gameState, commander); err != nil {
		return Context{}, err
	}
	if err := builder.addLegendSkills(gameState.Player.LegendSkills); err != nil {
		return Context{}, err
	}
	for _, source := range request.AdditionalSources {
		if source.Kind == "" {
			source.Kind = SourceAdditional
		}
		builder.addSource(source)
	}
	sort.Slice(context.Modifiers, func(left, right int) bool {
		return modifierKey(context.Modifiers[left]) < modifierKey(context.Modifiers[right])
	})
	sort.Slice(context.WaveModifiers, func(left, right int) bool {
		return waveModifierKey(context.WaveModifiers[left]) < waveModifierKey(context.WaveModifiers[right])
	})
	sort.Slice(context.SupportModifiers, func(left, right int) bool {
		return supportModifierKey(context.SupportModifiers[left]) < supportModifierKey(context.SupportModifiers[right])
	})
	sort.Slice(context.Skipped, func(left, right int) bool {
		return skippedEffectKey(context.Skipped[left]) < skippedEffectKey(context.Skipped[right])
	})
	return context, nil
}

func (builder contextBuilder) addLegendSkills(skills State.LegendSkillState) error {
	if !builder.target.LegendaryFight {
		return nil
	}
	if skills.ObservedAt.IsZero() {
		return fmt.Errorf("Hall of Legends skills have not been observed for this legendary fight")
	}
	catalog, err := builder.gameData.Catalog("legendskills")
	if err != nil {
		return fmt.Errorf("official Hall of Legends skill catalog is unavailable: %w", err)
	}
	for _, skillID := range skills.ActiveIDs {
		raw, exists := catalog.Find(strconv.FormatInt(skillID, 10))
		if !exists {
			continue
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		effectType, exists := record.String("effectType")
		if !exists {
			continue
		}
		if effectType == "additionalWave" {
			waves, exists := record.Float64("totalEffectValue")
			if !exists || waves <= 0 || !isFinite(waves) {
				continue
			}
			builder.context.WaveModifiers = append(builder.context.WaveModifiers, WaveModifier{
				SourceKind: SourceLegendSkill, SourceID: strconv.FormatInt(skillID, 10),
				SourceLabel: "Hall of Legends", EffectID: skillID, EffectName: effectType,
				Waves: int(math.Floor(waves)),
			})
			continue
		}
		if effectType == "additionalAttackToolAmountFlank" {
			bonus, exists := record.Float64("totalEffectValue")
			if !exists || bonus <= 0 || !isFinite(bonus) {
				continue
			}
			builder.context.LegendToolBonus = max(
				builder.context.LegendToolBonus,
				min(int64(math.Floor(bonus)), maximumLegendToolBonus),
			)
			continue
		}
		var lane Lane
		switch effectType {
		case "additionalUnitAmountOnFlank":
			lane = LaneFlank
		case "additionalUnitAmountOnFront":
			lane = LaneFront
		default:
			continue
		}
		percent, exists := record.Float64("totalEffectValue")
		if !exists || percent == 0 || !isFinite(percent) {
			continue
		}
		builder.context.Modifiers = append(builder.context.Modifiers, Modifier{
			SourceKind: SourceLegendSkill, SourceID: strconv.FormatInt(skillID, 10),
			SourceLabel: "Hall of Legends", EffectID: skillID, EffectName: effectType,
			Lane: lane, Percent: percent,
		})
	}
	return nil
}

// GeneralSkillsUnobservedError reports that a commander's assigned general
// has no observed skill set yet, so attack capacity cannot be resolved.
// Policies match it with errors.As and answer with a general-skills refresh
// instead of waiting: the observation only ever arrives from a "gie" pull,
// which the game never volunteers on its own.
type GeneralSkillsUnobservedError struct {
	GeneralID   int64
	CommanderID State.CommanderID
}

func (err *GeneralSkillsUnobservedError) Error() string {
	return fmt.Sprintf("general %d skills have not been observed for commander %d", err.GeneralID, err.CommanderID)
}

func (builder contextBuilder) addGeneralSkills(gameState State.GameState, commander State.CommanderState) error {
	if commander.GeneralID <= 0 {
		return nil
	}
	general, exists := gameState.Generals[commander.GeneralID]
	if !exists || general.ObservedAt.IsZero() {
		return &GeneralSkillsUnobservedError{GeneralID: commander.GeneralID, CommanderID: commander.ID}
	}
	catalog, err := builder.gameData.Catalog("generalSkills")
	if err != nil {
		return fmt.Errorf("official general skill catalog is unavailable: %w", err)
	}
	skillIDs := append([]int64(nil), general.ActiveSkillIDs...)
	sort.Slice(skillIDs, func(left, right int) bool { return skillIDs[left] < skillIDs[right] })
	seen := map[int64]struct{}{}
	for _, skillID := range skillIDs {
		if skillID <= 0 {
			continue
		}
		if _, duplicate := seen[skillID]; duplicate {
			continue
		}
		seen[skillID] = struct{}{}
		raw, exists := catalog.Find(strconv.FormatInt(skillID, 10))
		if !exists {
			return fmt.Errorf("general %d active skill %d is not in the official catalog", general.ID, skillID)
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return fmt.Errorf("decode general %d active skill %d: %w", general.ID, skillID, err)
		}
		if officialGeneralID, exists := record.Int64("generalID"); exists && officialGeneralID != general.ID {
			return fmt.Errorf("general %d active skill %d belongs to general %d", general.ID, skillID, officialGeneralID)
		}
		encoded, exists := record.String("effects")
		if !exists || strings.TrimSpace(encoded) == "" {
			continue
		}
		builder.addSource(EffectSource{
			Kind: SourceGeneralSkill, ID: fmt.Sprintf("%d:%d", general.ID, skillID),
			Label: fmt.Sprintf("general %d skill %d", general.ID, skillID), Effects: parseOfficialEffects(encoded, nil),
		})
	}
	return nil
}

// ResolveContext applies the official cap groups and rounds final lane and
// support capacities up the same way the game client does before a CRA command.
func (Resolver) ResolveContext(context Context) (Result, error) {
	context.Target = targetWithSemanticType(context.Target)
	context.Target = targetWithCastleType(context.Target)
	context.Target = targetWithInferredBaseCapacity(context.Target)
	if err := validateTarget(context.Target); err != nil {
		return Result{}, err
	}
	baseWaves := context.BaseWaves
	if baseWaves <= 0 {
		baseWaves = baseAttackWaves
	}
	additionalWaves := 0
	for _, modifier := range context.WaveModifiers {
		if modifier.Waves < 0 {
			return Result{}, fmt.Errorf("effect %d has an invalid wave bonus", modifier.EffectID)
		}
		if modifier.Waves > math.MaxInt-additionalWaves {
			return Result{}, fmt.Errorf("attack wave capacity is too large")
		}
		additionalWaves += modifier.Waves
	}
	groups := map[capGroupKey]*capGroupAccumulator{}
	for _, modifier := range context.Modifiers {
		if modifier.Lane != LaneFlank && modifier.Lane != LaneFront {
			return Result{}, fmt.Errorf("effect %d has an unsupported capacity lane %q", modifier.EffectID, modifier.Lane)
		}
		if !isFinite(modifier.Percent) {
			return Result{}, fmt.Errorf("effect %d has an invalid capacity bonus", modifier.EffectID)
		}
		key := capGroupKey{lane: modifier.Lane, capID: modifier.CapID}
		group := groups[key]
		if group == nil {
			group = &capGroupAccumulator{key: key}
			if maximum, exists := context.CapLimits[modifier.CapID]; exists && maximum > 0 {
				group.maximum = maximum
				group.hasMaximum = true
			}
			groups[key] = group
		}
		group.raw += modifier.Percent
	}

	keys := make([]capGroupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].lane != keys[right].lane {
			return keys[left].lane < keys[right].lane
		}
		return keys[left].capID < keys[right].capID
	})
	bonus := LaneBonus{}
	capGroups := make([]CapGroup, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		applied := group.raw
		if group.hasMaximum && applied > group.maximum {
			applied = group.maximum
		}
		capped := applied != group.raw
		capGroups = append(capGroups, CapGroup{
			Lane: key.lane, CapID: key.capID, RawPercent: group.raw, AppliedPercent: applied,
			MaximumPercent: group.maximum, Capped: capped,
		})
		switch key.lane {
		case LaneFlank:
			bonus.Left += applied
			bonus.Right += applied
		case LaneFront:
			bonus.Front += applied
		}
	}
	var capacity LaneCapacity
	var err error
	if context.Target.levelDerived {
		capacity, err = applyTargetLevelBonuses(context.Target.Level, bonus)
	} else {
		capacity, err = applyBonuses(context.Target.BaseCapacity, bonus)
	}
	if err != nil {
		return Result{}, err
	}
	baseToolCapacity := LaneCapacity{
		Left: basePvEFlankToolCapacity, Front: basePvEFrontToolCapacity, Right: basePvEFlankToolCapacity,
	}
	// Legendary PvP raises the base in all three sections. The Hall catalog's
	// additionalAttackToolAmountFlank value is applied separately below.
	if context.Target.PvP && context.Target.LegendaryFight {
		baseToolCapacity = LaneCapacity{
			Left:  baseLegendaryPvPFlankToolCapacity,
			Front: baseLegendaryPvPFrontToolCapacity,
			Right: baseLegendaryPvPFlankToolCapacity,
		}
	}
	legendToolBonus := int64(0)
	if context.Target.PvP && context.Target.LegendaryFight {
		legendToolBonus = min(max(int64(0), context.LegendToolBonus), maximumLegendToolBonus)
	}
	toolCapacity := LaneCapacity{
		Left:  baseToolCapacity.Left + legendToolBonus,
		Front: baseToolCapacity.Front,
		Right: baseToolCapacity.Right + legendToolBonus,
	}
	supportGroups := map[supportCapGroupKey]*supportCapGroupAccumulator{}
	for _, modifier := range context.SupportModifiers {
		if modifier.Kind != SupportModifierUnits && modifier.Kind != SupportModifierBoost {
			return Result{}, fmt.Errorf("effect %d has an unsupported support modifier kind %q", modifier.EffectID, modifier.Kind)
		}
		if !isFinite(modifier.Amount) {
			return Result{}, fmt.Errorf("effect %d has an invalid support capacity amount", modifier.EffectID)
		}
		key := supportCapGroupKey{kind: modifier.Kind, capID: modifier.CapID}
		group := supportGroups[key]
		if group == nil {
			group = &supportCapGroupAccumulator{key: key}
			if maximum, exists := context.CapLimits[modifier.CapID]; exists && maximum > 0 {
				group.maximum = maximum
				group.hasMaximum = true
			}
			supportGroups[key] = group
		}
		group.raw += modifier.Amount
	}
	supportKeys := make([]supportCapGroupKey, 0, len(supportGroups))
	for key := range supportGroups {
		supportKeys = append(supportKeys, key)
	}
	sort.Slice(supportKeys, func(left, right int) bool {
		if supportKeys[left].kind != supportKeys[right].kind {
			return supportKeys[left].kind < supportKeys[right].kind
		}
		return supportKeys[left].capID < supportKeys[right].capID
	})
	supportUnitAmount := float64(0)
	supportBoostPercent := float64(0)
	supportCapGroups := make([]SupportCapGroup, 0, len(supportKeys))
	for _, key := range supportKeys {
		group := supportGroups[key]
		applied := group.raw
		if group.hasMaximum && applied > group.maximum {
			applied = group.maximum
		}
		supportCapGroups = append(supportCapGroups, SupportCapGroup{
			Kind: key.kind, CapID: key.capID, RawAmount: group.raw, AppliedAmount: applied,
			MaximumAmount: group.maximum, Capped: applied != group.raw,
		})
		switch key.kind {
		case SupportModifierUnits:
			supportUnitAmount += applied
		case SupportModifierBoost:
			supportBoostPercent += applied
		}
	}
	supportCapacity, err := applySupportCapacity(supportUnitAmount, supportBoostPercent)
	if err != nil {
		return Result{}, err
	}
	return Result{
		SourceCastleID: context.SourceCastleID, CommanderID: context.CommanderID, Target: context.Target,
		BaseWaves: baseWaves, AdditionalWaves: additionalWaves, MaximumWaves: baseWaves + additionalWaves,
		WaveModifiers: append([]WaveModifier(nil), context.WaveModifiers...),
		BaseCapacity:  context.Target.BaseCapacity, BonusPercent: bonus, Capacity: capacity,
		BaseToolCapacity: baseToolCapacity, LegendToolBonus: legendToolBonus, ToolCapacity: toolCapacity,
		SupportUnitAmount: supportUnitAmount, SupportBoostPercent: supportBoostPercent, SupportCapacity: supportCapacity,
		SupportModifiers: append([]SupportModifier(nil), context.SupportModifiers...), SupportCapGroups: supportCapGroups,
		Modifiers: append([]Modifier(nil), context.Modifiers...), CapGroups: capGroups,
		Skipped: append([]SkippedEffect(nil), context.Skipped...),
	}, nil
}

type contextBuilder struct {
	gameData    *GameData.Store
	effects     *GameData.Catalog
	target      TargetContext
	context     *Context
	evaluatedAt time.Time
}

func (builder contextBuilder) addCastleBuildings(castle State.CastleState) error {
	catalog, err := builder.gameData.Catalog("buildings")
	if err != nil {
		return err
	}
	ids := make([]State.BuildingInstanceID, 0, len(castle.Buildings))
	for id := range castle.Buildings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, instanceID := range ids {
		building := castle.Buildings[instanceID]
		if building.DefinitionID <= 0 || building.Layer != "" && !building.Placed {
			continue
		}
		raw, exists := catalog.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
		if !exists {
			return fmt.Errorf("building %d: definition %d is not in the official catalog", instanceID, building.DefinitionID)
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return fmt.Errorf("building %d: %w", instanceID, err)
		}
		kind := SourceBuilding
		label := fmt.Sprintf("building definition %d", building.DefinitionID)
		if _, isDecoration := record.Float64("decoPoints"); isDecoration {
			kind = SourceDecoration
			label = fmt.Sprintf("decoration definition %d", building.DefinitionID)
		}
		builder.addSource(EffectSource{
			Kind: kind, ID: strconv.FormatInt(int64(instanceID), 10), Label: label,
			Effects: officialRecordEffects(record, nil),
		})
	}
	return nil
}

func (builder contextBuilder) addConstructionItems(castle State.CastleState) error {
	buildingIDs := make([]State.BuildingInstanceID, 0, len(castle.ConstructionSlots))
	for buildingID := range castle.ConstructionSlots {
		buildingIDs = append(buildingIDs, buildingID)
	}
	sort.Slice(buildingIDs, func(left, right int) bool { return buildingIDs[left] < buildingIDs[right] })
	for _, buildingID := range buildingIDs {
		slots := append([]State.ConstructionSlot(nil), castle.ConstructionSlots[buildingID]...)
		sort.Slice(slots, func(left, right int) bool {
			if slots[left].Slot != slots[right].Slot {
				return slots[left].Slot < slots[right].Slot
			}
			return slots[left].DefinitionID < slots[right].DefinitionID
		})
		for _, slot := range slots {
			if slot.DefinitionID <= 0 || !constructionSlotActive(slot, castle.ConstructionSlotsObservedAt, builder.evaluatedAt) {
				continue
			}
			effects, err := catalogEffects(builder.gameData, "constructionItems", int64(slot.DefinitionID), nil)
			if err != nil {
				return fmt.Errorf("construction item %d on building %d: %w", slot.DefinitionID, buildingID, err)
			}
			builder.addSource(EffectSource{
				Kind:    SourceConstructionItem,
				ID:      fmt.Sprintf("%d:%d", buildingID, slot.Slot),
				Label:   fmt.Sprintf("construction item %d", slot.DefinitionID),
				Effects: effects,
			})
		}
	}
	return nil
}

func (builder contextBuilder) addAttackDialogEffects(dialog State.AttackDialogState) {
	bySource := map[string]State.EquipmentEffects{}
	for _, effect := range dialog.ActiveEffects {
		if effect.EffectID <= 0 || len(effect.Values) == 0 {
			continue
		}
		source := strings.TrimSpace(effect.Source)
		if source == "" {
			source = "unknown"
		}
		bySource[source] = append(bySource[source], State.EquipmentEffect{
			WireID: effect.EffectID, DefinitionID: effect.EffectID, Values: append([]float64(nil), effect.Values...),
		})
	}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		kind, label := attackDialogSource(source)
		builder.addSource(EffectSource{
			Kind: kind, ID: source, Label: label, Effects: bySource[source],
		})
	}
}

func (builder contextBuilder) addCommanderEquipment(gameState State.GameState, commander State.CommanderState) (map[int64]int, error) {
	ids := sortedEquipmentIDs(commander.Equipment)
	setCounts := map[int64]int{}
	for _, id := range ids {
		item, exists := gameState.Inventory.Equipment[id]
		if !exists {
			return nil, fmt.Errorf("commander equipment %d is not observed", id)
		}
		if item.SetID > 0 {
			setCounts[item.SetID]++
		}
		effects := item.Effects
		if len(effects) == 0 {
			effects = optionalCatalogEffects(builder.gameData, "equipments", int64(item.DefinitionID), nil)
		}
		builder.addSource(EffectSource{
			Kind: SourceEquipment, ID: strconv.FormatInt(int64(id), 10),
			Label: fmt.Sprintf("equipment definition %d", item.DefinitionID), Effects: effects,
		})
	}
	return setCounts, nil
}

func (builder contextBuilder) addCommanderGems(gameState State.GameState, commander State.CommanderState, setCounts map[int64]int) error {
	ids := sortedGemIDs(commander.Gems)
	for _, id := range ids {
		gem, exists := gameState.Inventory.Gems[id]
		if !exists {
			return fmt.Errorf("commander gem %d is not observed", id)
		}
		if gem.SetID > 0 {
			setCounts[gem.SetID]++
		}
		effects := gem.Effects
		if len(effects) == 0 {
			effects = optionalCatalogEffects(builder.gameData, "gems", int64(gem.DefinitionID), nil)
		}
		builder.addSource(EffectSource{
			Kind: SourceGem, ID: strconv.FormatInt(int64(id), 10),
			Label: fmt.Sprintf("gem definition %d", gem.DefinitionID), Effects: effects,
		})
	}
	return nil
}

func (builder contextBuilder) addEquipmentSetBonuses(setCounts map[int64]int) error {
	if len(setCounts) == 0 {
		return nil
	}
	catalog, err := builder.gameData.Catalog("equipment_sets")
	if err != nil {
		return nil
	}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		setID, setOK := record.Int64("setID")
		needed, neededOK := record.Int64("neededItems")
		encoded, effectsOK := record.String("effects")
		if !setOK || !neededOK || !effectsOK || setCounts[setID] < int(needed) {
			continue
		}
		builder.addSource(EffectSource{
			Kind:  SourceEquipmentSet,
			ID:    fmt.Sprintf("%d:%d", setID, needed),
			Label: fmt.Sprintf("equipment set %d (%d pieces)", setID, needed),
			Effects: parseOfficialEffects(encoded, func(wireID int64) int64 {
				return normalEquipmentEffectID(builder.gameData, wireID)
			}),
		})
	}
	return nil
}

func (builder contextBuilder) addSource(source EffectSource) {
	for _, effect := range source.Effects {
		effectID := effect.DefinitionID
		if effectID <= 0 {
			effectID = effect.WireID
		}
		if effectID <= 0 {
			continue
		}
		raw, exists := builder.effects.Find(strconv.FormatInt(effectID, 10))
		if !exists {
			builder.context.Skipped = append(builder.context.Skipped, SkippedEffect{
				SourceKind: source.Kind, SourceID: source.ID, EffectID: effectID,
				Reason: "effect is not in the official catalog",
			})
			continue
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			builder.context.Skipped = append(builder.context.Skipped, SkippedEffect{
				SourceKind: source.Kind, SourceID: source.ID, EffectID: effectID,
				Reason: "official effect record could not be decoded",
			})
			continue
		}
		name := effectName(record)
		if effectTypeID, exists := record.Int64("effectTypeID"); exists && effectTypeID == waveEffectTypeID {
			if reason := targetMismatch(record, name, builder.target); reason != "" {
				builder.context.Skipped = append(builder.context.Skipped, SkippedEffect{
					SourceKind: source.Kind, SourceID: source.ID, EffectID: effectID, EffectName: name, Reason: reason,
				})
				continue
			}
			waves := effectMagnitude(effect.Values)
			if waves <= 0 || !isFinite(waves) {
				continue
			}
			builder.context.WaveModifiers = append(builder.context.WaveModifiers, WaveModifier{
				SourceKind: source.Kind, SourceID: source.ID, SourceLabel: source.Label,
				EffectID: effectID, EffectName: name, Waves: int(math.Floor(waves)),
			})
			continue
		}
		if kind, supportEffect := supportModifierKind(record); supportEffect {
			if reason := targetMismatch(record, name, builder.target); reason != "" {
				builder.context.Skipped = append(builder.context.Skipped, SkippedEffect{
					SourceKind: source.Kind, SourceID: source.ID, EffectID: effectID, EffectName: name, Reason: reason,
				})
				continue
			}
			amount := effectMagnitude(effect.Values)
			if amount == 0 || !isFinite(amount) {
				continue
			}
			capID, _ := record.Int64("capID")
			builder.context.SupportModifiers = append(builder.context.SupportModifiers, SupportModifier{
				SourceKind: source.Kind, SourceID: source.ID, SourceLabel: source.Label,
				EffectID: effectID, EffectName: name, Kind: kind, CapID: capID, Amount: amount,
			})
			continue
		}
		lane, capacityEffect := capacityLane(record, name)
		if !capacityEffect {
			continue
		}
		if reason := targetMismatch(record, name, builder.target); reason != "" {
			builder.context.Skipped = append(builder.context.Skipped, SkippedEffect{
				SourceKind: source.Kind, SourceID: source.ID, EffectID: effectID, EffectName: name, Reason: reason,
			})
			continue
		}
		percent := effectMagnitude(effect.Values)
		if percent == 0 || !isFinite(percent) {
			continue
		}
		capID, _ := record.Int64("capID")
		builder.context.Modifiers = append(builder.context.Modifiers, Modifier{
			SourceKind: source.Kind, SourceID: source.ID, SourceLabel: source.Label,
			EffectID: effectID, EffectName: name, Lane: lane, CapID: capID, Percent: percent,
		})
	}
}

func catalogEffects(gameData *GameData.Store, collection string, id int64, resolve func(int64) int64) (State.EquipmentEffects, error) {
	catalog, err := gameData.Catalog(collection)
	if err != nil {
		return nil, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return nil, fmt.Errorf("definition %d is not in the official catalog", id)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return nil, err
	}
	return officialRecordEffects(record, resolve), nil
}

func optionalCatalogEffects(
	gameData *GameData.Store,
	collection string,
	id int64,
	resolve func(int64) int64,
) State.EquipmentEffects {
	if id <= 0 {
		return nil
	}
	catalog, err := gameData.Catalog(collection)
	if err != nil {
		return nil
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return nil
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return nil
	}
	return officialRecordEffects(record, resolve)
}

func officialRecordEffects(record GameData.Record, resolve func(int64) int64) State.EquipmentEffects {
	encoded, exists := record.String("effects")
	if !exists || strings.TrimSpace(encoded) == "" {
		return nil
	}
	return parseOfficialEffects(encoded, resolve)
}

func constructionSlotActive(slot State.ConstructionSlot, observedAt time.Time, evaluatedAt time.Time) bool {
	if slot.RemainingSec == nil {
		return true
	}
	if *slot.RemainingSec <= 0 {
		return false
	}
	if observedAt.IsZero() || evaluatedAt.IsZero() || evaluatedAt.Before(observedAt) {
		return true
	}
	elapsed := int64(evaluatedAt.Sub(observedAt) / time.Second)
	return elapsed < int64(*slot.RemainingSec)
}

func attackDialogSource(source string) (SourceKind, string) {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case "BG":
		return SourceBuilding, "active building effects"
	case "CI":
		return SourceConstructionItem, "active construction-item effects"
	case "DE":
		return SourceDecoration, "active decoration effects"
	case "TL":
		return SourceTitle, "active title effects"
	case "RH":
		return SourceResearch, "active research effects"
	case "AB":
		return SourceAllianceBonus, "active alliance effects"
	case "AC":
		return SourceAllianceBonus, "active alliance-coat effects"
	case "GE":
		return SourceGlobalEffect, "active global effects"
	case "PS":
		return SourceAccountEffect, "active account effects"
	default:
		return SourceAttackDialog, "active pre-attack effects (" + source + ")"
	}
}

func parseOfficialEffects(encoded string, resolve func(int64) int64) State.EquipmentEffects {
	result := State.EquipmentEffects{}
	for _, entry := range strings.Split(encoded, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "&", 2)
		if len(parts) != 2 {
			continue
		}
		wireID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || wireID <= 0 {
			continue
		}
		values := []float64{}
		for _, rawValue := range strings.FieldsFunc(parts[1], func(value rune) bool {
			return value == '+' || value == '#'
		}) {
			value, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
			if err == nil && isFinite(value) {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			continue
		}
		effectID := wireID
		if resolve != nil {
			effectID = resolve(wireID)
		}
		result = append(result, State.EquipmentEffect{WireID: wireID, DefinitionID: effectID, Values: values})
	}
	return result
}

func normalEquipmentEffectID(gameData *GameData.Store, wireID int64) int64 {
	catalog, err := gameData.Catalog("equipment_effects")
	if err != nil {
		return wireID
	}
	raw, exists := catalog.Find(strconv.FormatInt(wireID, 10))
	if !exists {
		return wireID
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return wireID
	}
	effectID, exists := record.Int64("effectID")
	if !exists || effectID <= 0 {
		return wireID
	}
	return effectID
}

func loadCapLimits(gameData *GameData.Store) map[int64]float64 {
	limits := map[int64]float64{}
	catalog, err := gameData.Catalog("effectCaps")
	if err != nil {
		return limits
	}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		capID, capOK := record.Int64("capID")
		maximum, maximumOK := record.Float64("maxTotalBonus")
		if capOK && maximumOK && maximum > 0 && isFinite(maximum) {
			limits[capID] = maximum
		}
	}
	return limits
}

func capacityLane(record GameData.Record, _ string) (Lane, bool) {
	effectTypeID, exists := record.Int64("effectTypeID")
	if !exists {
		return "", false
	}
	switch effectTypeID {
	case flankEffectTypeID:
		return LaneFlank, true
	case frontEffectTypeID:
		return LaneFront, true
	default:
		return "", false
	}
}

func supportModifierKind(record GameData.Record) (SupportModifierKind, bool) {
	effectTypeID, exists := record.Int64("effectTypeID")
	if !exists {
		return "", false
	}
	switch effectTypeID {
	case supportUnitsEffectTypeID:
		return SupportModifierUnits, true
	case supportBoostEffectTypeID:
		return SupportModifierBoost, true
	default:
		return "", false
	}
}

func targetMismatch(record GameData.Record, name string, target TargetContext) string {
	hasOfficialTargetRestriction := false
	if areaTypes, exists := record.String("areaTypeID"); exists && strings.TrimSpace(areaTypes) != "" {
		hasOfficialTargetRestriction = true
		if target.CastleTypeID <= 0 {
			return "target castle type is unknown"
		}
		if !containsAreaType(parseAreaTypes(areaTypes), target.CastleTypeID) {
			return "effect does not apply to the target castle type"
		}
	}
	if isPvP, exists := record.Int64("isPvPFight"); exists {
		hasOfficialTargetRestriction = true
		if (isPvP != 0) != target.PvP {
			return "effect does not apply to this fight type"
		}
	}
	if hasOfficialTargetRestriction {
		return ""
	}
	lower := strings.ToLower(name)
	if strings.Contains(lower, "pvp") && !target.PvP {
		return "effect is restricted to PvP targets"
	}
	if strings.Contains(lower, "pve") && target.PvP {
		return "effect is restricted to PvE targets"
	}
	return ""
}

func parseAreaTypes(raw string) []int {
	result := []int{}
	for _, value := range strings.FieldsFunc(raw, func(character rune) bool {
		return character == ',' || character == '#'
	}) {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && parsed > 0 {
			result = append(result, parsed)
		}
	}
	return result
}

func containsAreaType(areaTypes []int, wanted int) bool {
	for _, areaType := range areaTypes {
		if areaType == wanted {
			return true
		}
	}
	return false
}

func effectName(record GameData.Record) string {
	if name, exists := record.String("name"); exists {
		return name
	}
	name, _ := record.String("Name")
	return name
}

func effectMagnitude(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	if len(values)%2 == 0 {
		paired := true
		for index := 0; index < len(values); index += 2 {
			if values[index] < 1 || math.Trunc(values[index]) != values[index] {
				paired = false
				break
			}
		}
		if paired {
			best := values[1]
			for index := 3; index < len(values); index += 2 {
				if values[index] > best {
					best = values[index]
				}
			}
			return best
		}
	}
	return values[len(values)-1]
}

func sortedEquipmentIDs(equipment map[string]State.EquipmentInstanceID) []State.EquipmentInstanceID {
	seen := map[State.EquipmentInstanceID]struct{}{}
	result := make([]State.EquipmentInstanceID, 0, len(equipment))
	for _, id := range equipment {
		if id > 0 {
			if _, duplicate := seen[id]; !duplicate {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func resolvedSourceCastle(gameState State.GameState, requested State.CastleID) (State.CastleID, State.CastleState, error) {
	if requested > 0 {
		castle, exists := gameState.Castles[requested]
		if !exists {
			return 0, State.CastleState{}, fmt.Errorf("source castle %d is not in the current player state", requested)
		}
		return requested, castle, nil
	}
	for castleID, castle := range gameState.Castles {
		if castle.Focused {
			return castleID, castle, nil
		}
	}
	return 0, State.CastleState{}, fmt.Errorf("source castle id is required when no castle is focused")
}

func sortedGemIDs(gems map[string]State.GemInstanceID) []State.GemInstanceID {
	seen := map[State.GemInstanceID]struct{}{}
	result := make([]State.GemInstanceID, 0, len(gems))
	for _, id := range gems {
		if id != 0 {
			if _, duplicate := seen[id]; !duplicate {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

type capGroupKey struct {
	lane  Lane
	capID int64
}

type capGroupAccumulator struct {
	key        capGroupKey
	raw        float64
	maximum    float64
	hasMaximum bool
}

type supportCapGroupKey struct {
	kind  SupportModifierKind
	capID int64
}

type supportCapGroupAccumulator struct {
	key        supportCapGroupKey
	raw        float64
	maximum    float64
	hasMaximum bool
}

func applyBonuses(base LaneCapacity, bonus LaneBonus) (LaneCapacity, error) {
	left, err := applyPercent(base.Left, bonus.Left)
	if err != nil {
		return LaneCapacity{}, err
	}
	front, err := applyPercent(base.Front, bonus.Front)
	if err != nil {
		return LaneCapacity{}, err
	}
	right, err := applyPercent(base.Right, bonus.Right)
	if err != nil {
		return LaneCapacity{}, err
	}
	return LaneCapacity{Left: left, Front: front, Right: right}, nil
}

func applyTargetLevelBonuses(targetLevel int, bonus LaneBonus) (LaneCapacity, error) {
	maxAttackers := maxAttackersForTargetLevel(targetLevel)
	if maxAttackers <= 0 {
		return LaneCapacity{}, fmt.Errorf("target level is required")
	}
	base := BaseCapacityForTargetLevel(targetLevel)
	left, err := applyRawPercent(float64(maxAttackers)*0.2, bonus.Left)
	if err != nil {
		return LaneCapacity{}, err
	}
	front, err := applyRawPercent(float64(base.Front), bonus.Front)
	if err != nil {
		return LaneCapacity{}, err
	}
	right, err := applyRawPercent(float64(maxAttackers)*0.2, bonus.Right)
	if err != nil {
		return LaneCapacity{}, err
	}
	return LaneCapacity{Left: left, Front: front, Right: right}, nil
}

func applyPercent(base int64, percent float64) (int64, error) {
	return applyRawPercent(float64(base), percent)
}

func applyRawPercent(base float64, percent float64) (int64, error) {
	if base < 0 || !isFinite(base) {
		return 0, fmt.Errorf("lane base capacity cannot be negative")
	}
	if !isFinite(percent) {
		return 0, fmt.Errorf("lane capacity bonus is invalid")
	}
	capacity := math.Ceil(base * math.Max(0, 100+percent) / 100)
	if capacity > float64(math.MaxInt64) {
		return 0, fmt.Errorf("lane capacity is too large")
	}
	return int64(capacity), nil
}

func applySupportCapacity(units float64, percent float64) (int64, error) {
	if !isFinite(units) || units < 0 {
		return 0, fmt.Errorf("support unit capacity is invalid")
	}
	if !isFinite(percent) {
		return 0, fmt.Errorf("support capacity bonus is invalid")
	}
	capacity := math.Ceil(units * math.Max(0, 100+percent) / 100)
	if capacity > float64(math.MaxInt64) {
		return 0, fmt.Errorf("support capacity is too large")
	}
	return int64(capacity), nil
}

func validateTarget(target TargetContext) error {
	switch target.TargetType {
	case "":
	case TargetTypeBerimondTower:
		if target.Map == nil {
			return fmt.Errorf("Berimond tower target requires map identity")
		}
		if target.Map.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
			return fmt.Errorf("Berimond tower target requires kingdom %d", GameData.BerimondKingdomID)
		}
		if target.Map.TypeID != BerimondTowerMapTypeID {
			return fmt.Errorf("Berimond tower target requires map type %d", BerimondTowerMapTypeID)
		}
		if target.CastleTypeID != BerimondTowerMapTypeID {
			return fmt.Errorf("Berimond tower target requires castle type %d", BerimondTowerMapTypeID)
		}
		if target.PvP {
			return fmt.Errorf("Berimond tower target must be a PvE fight")
		}
	default:
		return fmt.Errorf("unsupported attack target type %q", target.TargetType)
	}
	if target.CastleTypeID > 0 && target.Map != nil && target.Map.TypeID > 0 && target.Map.TypeID != target.CastleTypeID {
		return fmt.Errorf("target castle type %d does not match map type %d", target.CastleTypeID, target.Map.TypeID)
	}
	if target.CastleTypeID > 0 && target.AreaTypeID > 0 && target.AreaTypeID != target.CastleTypeID {
		return fmt.Errorf("target castle type %d does not match legacy area type %d", target.CastleTypeID, target.AreaTypeID)
	}
	if target.BaseCapacity.Left < 0 || target.BaseCapacity.Front < 0 || target.BaseCapacity.Right < 0 {
		return fmt.Errorf("target base capacities cannot be negative")
	}
	if target.BaseCapacity.Left == 0 && target.BaseCapacity.Front == 0 && target.BaseCapacity.Right == 0 {
		return fmt.Errorf("target base capacity is required")
	}
	return nil
}

func targetWithSemanticType(target TargetContext) TargetContext {
	if target.TargetType == "" && target.Map != nil &&
		target.Map.KingdomID == State.KingdomID(GameData.BerimondKingdomID) &&
		target.Map.TypeID == BerimondTowerMapTypeID {
		target.TargetType = TargetTypeBerimondTower
	}
	return target
}

func targetWithCastleType(target TargetContext) TargetContext {
	if target.CastleTypeID <= 0 {
		if target.Map != nil && target.Map.TypeID > 0 {
			target.CastleTypeID = target.Map.TypeID
		} else if target.AreaTypeID > 0 {
			target.CastleTypeID = target.AreaTypeID
		}
	}
	if target.AreaTypeID <= 0 {
		target.AreaTypeID = target.CastleTypeID
	}
	return target
}

func targetWithInferredBaseCapacity(target TargetContext) TargetContext {
	if target.Level <= 0 && target.Map != nil {
		target.Level = target.Map.Level
		if target.Level <= 0 && target.Map.TypeID == 2 {
			target.Level = DungeonTargetLevel(target.Map.VictoryCount, target.Map.KingdomID)
		}
	}
	if target.BaseCapacity == (LaneCapacity{}) && target.Level > 0 {
		target.BaseCapacity = BaseCapacityForTargetLevel(target.Level)
		target.levelDerived = true
	}
	return target
}

func targetWithAttackDialog(target TargetContext, dialog State.AttackDialogState) TargetContext {
	if target.Map != nil || dialog.Target.TypeID <= 0 {
		return target
	}
	target.Map = &MapTarget{
		KingdomID: dialog.KingdomID, TypeID: dialog.Target.TypeID,
		X: dialog.Target.X, Y: dialog.Target.Y, ObjectID: dialog.Target.ObjectID,
	}
	return target
}

func validateAttackDialogTarget(target TargetContext, dialog State.AttackDialogState) error {
	requestedTypeID := target.CastleTypeID
	if requestedTypeID <= 0 && target.Map != nil {
		requestedTypeID = target.Map.TypeID
	}
	if requestedTypeID <= 0 {
		requestedTypeID = target.AreaTypeID
	}
	if requestedTypeID > 0 && dialog.Target.TypeID > 0 && requestedTypeID != dialog.Target.TypeID {
		return fmt.Errorf("the pre-attack dialog belongs to target castle type %d, not %d", dialog.Target.TypeID, requestedTypeID)
	}
	if target.Map == nil || dialog.Target.TypeID <= 0 {
		return nil
	}
	if target.Map.KingdomID != dialog.KingdomID || target.Map.X != dialog.Target.X || target.Map.Y != dialog.Target.Y {
		return fmt.Errorf(
			"the pre-attack dialog belongs to target %d:%d:%d, not %d:%d:%d",
			dialog.KingdomID, dialog.Target.X, dialog.Target.Y,
			target.Map.KingdomID, target.Map.X, target.Map.Y,
		)
	}
	if target.Map.ObjectID > 0 && dialog.Target.ObjectID > 0 && target.Map.ObjectID != dialog.Target.ObjectID {
		return fmt.Errorf("the pre-attack dialog target object %d does not match %d", dialog.Target.ObjectID, target.Map.ObjectID)
	}
	return nil
}

func modifierKey(modifier Modifier) string {
	return strings.Join([]string{
		string(modifier.Lane), strconv.FormatInt(modifier.CapID, 10), string(modifier.SourceKind),
		modifier.SourceID, strconv.FormatInt(modifier.EffectID, 10), strconv.FormatFloat(modifier.Percent, 'f', -1, 64),
	}, "\x00")
}

func waveModifierKey(modifier WaveModifier) string {
	return strings.Join([]string{
		string(modifier.SourceKind), modifier.SourceID, strconv.FormatInt(modifier.EffectID, 10), strconv.Itoa(modifier.Waves),
	}, "\x00")
}

func supportModifierKey(modifier SupportModifier) string {
	return strings.Join([]string{
		string(modifier.Kind), strconv.FormatInt(modifier.CapID, 10), string(modifier.SourceKind),
		modifier.SourceID, strconv.FormatInt(modifier.EffectID, 10), strconv.FormatFloat(modifier.Amount, 'f', -1, 64),
	}, "\x00")
}

func skippedEffectKey(effect SkippedEffect) string {
	return strings.Join([]string{
		string(effect.SourceKind), effect.SourceID, strconv.FormatInt(effect.EffectID, 10), effect.Reason,
	}, "\x00")
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
