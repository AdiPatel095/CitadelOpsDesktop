package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	nomadEventID           = 72
	nomadCampTypeID        = 27
	samuraiEventID         = 80
	samuraiCampTypeID      = 29
	nomadCampCount         = 4
	fixedNomadRadius       = 50
	defaultNomadMapRefresh = 300
)

type AutoNomadPolicy struct{}

type autoNomadSettings struct {
	Version               int                      `json:"version"`
	SourceCastleID        State.CastleID           `json:"sourceCastleId"`
	LegacyPresetID        string                   `json:"presetId,omitempty"`
	NomadPresetID         string                   `json:"nomadPresetId"`
	SamuraiPresetID       string                   `json:"samuraiPresetId"`
	NomadDifficultyID     int64                    `json:"nomadDifficultyId"`
	SamuraiDifficultyID   int64                    `json:"samuraiDifficultyId"`
	ScoreTarget           int64                    `json:"scoreTarget"`
	MinimumRemainingSec   int64                    `json:"minimumRemainingSec"`
	CheckIntervalSec      int                      `json:"checkIntervalSec"`
	MapRefreshIntervalSec int                      `json:"mapRefreshIntervalSec"`
	DailyAttackLimit      int64                    `json:"dailyAttackLimit"`
	HorseTravelBoostID    int                      `json:"horseTravelBoostId"`
	SkipCooldowns         bool                     `json:"skipCooldowns"`
	TimeSkipReserve       map[string]int64         `json:"timeSkipReserve"`
	RBCTest               autoNomadRBCTestSettings `json:"rbcTest"`
}

type autoNomadRBCTestSettings struct {
	Enabled bool   `json:"enabled"`
	RunID   string `json:"runId"`
	TargetX int    `json:"targetX"`
	TargetY int    `json:"targetY"`
}

type nomadCampCandidate struct {
	Observation State.MapObservation
	Definition  GameData.EventCampDefinition
}

func NewAutoNomadPolicy() *AutoNomadPolicy { return &AutoNomadPolicy{} }

func (*AutoNomadPolicy) ID() string { return "autoNomad" }

func (*AutoNomadPolicy) EnabledKey() string { return "auto_nomad" }

func (*AutoNomadPolicy) WakeDomains() []string {
	return []string{"attacks", "map-event-camp", "movements", "commanders", "units", "events", "event-scores", "nomad-camps", "tower-cooldowns", "currencies", "attack_dialog", "achievements"}
}

func (*AutoNomadPolicy) WakeSections() []string {
	return []string{"automation.autoNomad", AttackPresets.ConfigurationSection, commanderFeatureSection}
}

func (*AutoNomadPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {

	settings := autoNomadSettings{
		MinimumRemainingSec: 1800, CheckIntervalSec: 30, MapRefreshIntervalSec: defaultNomadMapRefresh,
		TimeSkipReserve: map[string]int64{}, HorseTravelBoostID: -1,
	}
	if !decodeSection(snapshot.Configuration, "automation.autoNomad", &settings) {
		return nomadWaiting(snapshot.Now, "Auto Nomad/Samurai is not configured"), nil
	}
	settings.LegacyPresetID = strings.TrimSpace(settings.LegacyPresetID)
	settings.NomadPresetID = strings.TrimSpace(settings.NomadPresetID)
	settings.SamuraiPresetID = strings.TrimSpace(settings.SamuraiPresetID)
	if settings.NomadPresetID == "" {
		settings.NomadPresetID = settings.LegacyPresetID
	}
	if settings.SamuraiPresetID == "" {
		settings.SamuraiPresetID = settings.LegacyPresetID
	}
	settings.RBCTest.RunID = strings.TrimSpace(settings.RBCTest.RunID)
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return nomadWaiting(snapshot.Now, "Choose a supported horse travel boost"), nil
	}
	if settings.RBCTest.Enabled {
		return evaluateAutoNomadRBCTest(snapshot, settings)
	}
	if settings.SourceCastleID <= 0 || settings.NomadPresetID == "" || settings.SamuraiPresetID == "" || settings.ScoreTarget <= 0 ||
		settings.NomadDifficultyID <= 0 || settings.SamuraiDifficultyID <= 0 {
		return nomadWaiting(snapshot.Now, "Choose a source castle, both event attack presets, both event difficulties, and score target"), nil
	}
	if invalidTimeSkipReserve(settings.TimeSkipReserve) {
		return nomadWaiting(snapshot.Now, "Time-skip reserves cannot be negative"), nil
	}

	score, found := activeNomadEventScore(snapshot.State, snapshot.Now)
	if !found {
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{nomadEventID, samuraiEventID}, "Nomad or Samurai event",
		); locked {
			return decision, nil
		}
		return nomadWaiting(snapshot.Now, "No Nomad or Samurai scalable event is active"), nil
	}
	targetTypeID, _ := nomadTargetType(score.EventID)
	difficultyID := configuredNomadDifficulty(settings, score.EventID)
	if snapshot.GameData == nil {
		return nomadWaiting(snapshot.Now, "Official event data is unavailable"), nil
	}
	difficulty, valid := snapshot.GameData.ScalableEvent(score.EventID, difficultyID)
	if !valid {
		return nomadWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not valid for event %d", difficultyID, score.EventID)), nil
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !snapshot.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return nomadWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not unlocked by this player's achievements", difficultyID)), nil
	}
	if score.DifficultyID <= 0 {
		arguments, _ := json.Marshal(map[string]any{"eventId": score.EventID, "difficultyId": difficultyID})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Start %s at difficulty %d", nomadEventName(score.EventID), difficultyID),
			NextCheckAt: snapshot.Now.Add(2 * time.Second),
			Request:     &Intent.Request{Name: "nomad.difficulty.select", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if score.DifficultyID != difficultyID {
		return nomadWaiting(snapshot.Now, fmt.Sprintf(
			"The active %s run already uses difficulty %d; configured difficulty %d applies to the next run",
			nomadEventName(score.EventID), score.DifficultyID, difficultyID,
		)), nil
	}
	if score.PlayerScore >= settings.ScoreTarget {
		return Decision{
			Status: "complete", Detail: fmt.Sprintf("Score target reached: %d / %d", score.PlayerScore, settings.ScoreTarget),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
			Metrics:     map[string]float64{"score": float64(score.PlayerScore), "scoreTarget": float64(settings.ScoreTarget)},
		}, nil
	}
	if remaining := invasionEventRemaining(score, snapshot.Now); remaining >= 0 && remaining <= max(0, settings.MinimumRemainingSec) {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Event has %d seconds remaining; no new attacks will launch", remaining),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
		}, nil
	}

	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists {
		return nomadWaiting(snapshot.Now, fmt.Sprintf("Source castle %d is unavailable", settings.SourceCastleID)), nil
	}
	if source.KingdomID != 0 {
		return nomadWaiting(snapshot.Now, "Nomad and Samurai camp attacks require a Great Empire castle"), nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	preset, exists := AttackPresets.Find(document, configuredNomadPresetID(settings, score.EventID))
	if !exists {
		return nomadWaiting(snapshot.Now, fmt.Sprintf("The selected %s attack preset no longer exists", nomadEventName(score.EventID))), nil
	}
	progression := snapshot.GameData.EventCampProgression(score.EventID, score.DifficultyID, targetTypeID)
	if len(progression) == 0 {
		return nomadWaiting(snapshot.Now, "Official regular-camp progression data is unavailable"), nil
	}
	maximumVictoryCount := progression[len(progression)-1].VictoryCount
	metrics := map[string]float64{
		"score": float64(score.PlayerScore), "scoreTarget": float64(settings.ScoreTarget),
		"maximumVictoryCount": float64(maximumVictoryCount),
	}
	dailyAllowance, blocked := dailyAttackLimitAllowance(
		snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics,
	)
	if blocked != nil {
		return *blocked, nil
	}
	if locked := snapshot.State.NomadCamps.LockedTarget; locked != nil &&
		locked.SourceCastleID == source.ID && locked.EventID == score.EventID && locked.DifficultyID == score.DifficultyID &&
		locked.TypeID == targetTypeID {
		key := fmt.Sprintf("%d:%d:%d", locked.KingdomID, locked.X, locked.Y)
		if cooldown, found := snapshot.State.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
			arguments, _ := json.Marshal(map[string]any{
				"kingdomId": locked.KingdomID, "x1": locked.X, "y1": locked.Y, "x2": locked.X, "y2": locked.Y,
			})
			return Decision{
				Status: "ready", Detail: fmt.Sprintf("Refresh locked camp %d:%d immediately after its confirmed victory", locked.X, locked.Y),
				NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
				Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
			}, nil
		}
		if observation, found := snapshot.State.LookupMapObservation(locked.KingdomID, fmt.Sprintf("%d:%d", locked.X, locked.Y)); found &&
			observation.TypeID == locked.TypeID && observation.EventCampID == locked.EventCampID {
			remaining := nomadCampCooldownRemaining(snapshot.State, observation, snapshot.Now)
			if remaining > 0 {
				metrics["cooldownRemaining"] = float64(remaining)
				if !settings.SkipCooldowns {
					return Decision{
						Status: "waiting", Detail: fmt.Sprintf("Locked camp cooldown has %d seconds remaining; time skips are disabled", remaining),
						NextCheckAt: snapshot.Now.Add(time.Duration(remaining) * time.Second), Metrics: metrics,
					}, nil
				}
				return Decision{
					Status: "ready", Detail: fmt.Sprintf("Clear locked camp %d:%d before the next chained arrival", locked.X, locked.Y),
					NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
					Request: &Intent.Request{
						Name: "nomad.cooldown.minute_skip", Arguments: nomadMinuteSkipArguments(observation, settings.TimeSkipReserve),
					}, ReevaluateOnSuccess: true,
				}, nil
			}
		}
	}

	refreshInterval := nomadMapRefreshInterval(settings.MapRefreshIntervalSec)
	lastScan := snapshot.State.NomadCamps.LastScannedAt[source.ID]
	if lastScan.IsZero() || snapshot.Now.Sub(lastScan) >= refreshInterval {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "radius": fixedNomadRadius, "scanStartedAt": snapshot.Now,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Discover the four %s camps around %s", nomadEventName(score.EventID), invasionCastleName(source)),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "nomad.map.scan", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}

	camps := nomadCampCandidates(snapshot.State, snapshot.GameData, source, score.EventID, score.DifficultyID, targetTypeID, lastScan)
	metrics["knownCamps"] = float64(len(camps))
	if len(camps) < nomadCampCount {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("Found %d of the expected %d regular camps", len(camps), nomadCampCount),
			NextCheckAt: lastScan.Add(refreshInterval), Metrics: metrics,
		}, nil
	}
	camps = camps[:nomadCampCount]
	if pending, found := pendingNomadCampRefresh(snapshot.State, camps); found {
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": pending.KingdomID, "x1": pending.X, "y1": pending.Y, "x2": pending.X, "y2": pending.Y,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Refresh camp %d:%d after its confirmed victory", pending.X, pending.Y),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}

	maxedCount := 0
	for _, camp := range camps {
		if camp.Observation.EventCampVictoryCount >= maximumVictoryCount {
			maxedCount++
		}
	}
	metrics["maxedCamps"] = float64(maxedCount)
	metrics["activeAttacks"] = float64(activeNomadCampAttackCount(snapshot.State, source.ID, camps, snapshot.Now))
	commanderIDs, commandersRestricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoNomad")
	availableCommanders := availableNomadCommanders(snapshot.State, commanderIDs, commandersRestricted)
	metrics["availableCommanders"] = float64(len(availableCommanders))

	if maxedCount < nomadCampCount {
		if len(availableCommanders) == 0 {
			return nomadCommanderWaiting(snapshot.Now, commandersRestricted, metrics), nil
		}
		target := lowestNomadCamp(camps, maximumVictoryCount)
		if remaining := nomadCampCooldownRemaining(snapshot.State, target.Observation, snapshot.Now); remaining > 0 {
			if settings.SkipCooldowns {
				if responseGatedDungeonCooldownCount(snapshot.State, settings.TimeSkipReserve, int64(remaining)) < 1 {
					return Decision{
						Status: "waiting", Detail: fmt.Sprintf("Available time skips cannot clear camp %d:%d while preserving reserves", target.Observation.X, target.Observation.Y),
						NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
					}, nil
				}
				return Decision{
					Status: "ready", Detail: fmt.Sprintf("Clear camp %d:%d cooldown and continue leveling the four camps", target.Observation.X, target.Observation.Y),
					NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
					Request: &Intent.Request{
						Name: "nomad.cooldown.minute_skip", Arguments: nomadMinuteSkipArguments(target.Observation, settings.TimeSkipReserve),
					}, ReevaluateOnSuccess: true,
				}, nil
			}
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Camp %d:%d is available in %d seconds", target.Observation.X, target.Observation.Y, remaining),
				NextCheckAt: snapshot.Now.Add(time.Duration(remaining) * time.Second), Metrics: metrics,
			}, nil
		}
		launchCommanders, limitedPreset, err := availableNomadPresetCommanders(
			snapshot, source, target, preset, availableCommanders, len(availableCommanders), false,
		)
		if err != nil {
			if decision, refresh := generalSkillsRefreshDecision(err, snapshot.Now, metrics); refresh {
				return decision, nil
			}
			return nomadWaiting(snapshot.Now, fmt.Sprintf("Cannot calculate %s inventory requirements: %v", preset.Name, err)), nil
		}
		metrics["presetCopies"] = float64(len(launchCommanders))
		if len(launchCommanders) == 0 {
			return nomadPresetWaiting(snapshot.Now, limitedPreset, source, snapshot.GameData, metrics), nil
		}
		return nomadAttackDecision(snapshot.Now, score, settings, source, preset, target, launchCommanders, "level", maximumVictoryCount, metrics), nil
	}

	weakest := weakestNomadCamp(camps)
	locked := snapshot.State.NomadCamps.LockedTarget
	if !validNomadLock(locked, score, source, camps, maximumVictoryCount) {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "eventId": score.EventID, "difficultyId": score.DifficultyID,
			"kingdomId": weakest.Observation.KingdomID, "targetTypeId": weakest.Observation.TypeID,
			"targetX": weakest.Observation.X, "targetY": weakest.Observation.Y,
			"eventCampId": weakest.Observation.EventCampID, "victoryCount": weakest.Observation.EventCampVictoryCount,
			"defenseScore": nomadCampDefenseScore(weakest), "eventEndsAt": nomadEventEndsAt(score),
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Lock weakest maxed camp at %d:%d", weakest.Observation.X, weakest.Observation.Y),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "nomad.target.lock", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	target, found := lockedNomadCandidate(*locked, camps)
	if !found {
		return nomadWaiting(snapshot.Now, "The locked camp is no longer present in the fresh four-camp scan"), nil
	}
	metrics["lockedTargetX"] = float64(target.Observation.X)
	metrics["lockedTargetY"] = float64(target.Observation.Y)
	remainingCooldown := nomadCampCooldownRemaining(snapshot.State, target.Observation, snapshot.Now)
	metrics["cooldownRemaining"] = float64(remainingCooldown)
	if remainingCooldown > 0 {
		if !settings.SkipCooldowns {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Locked camp cooldown has %d seconds remaining; time skips are disabled", remainingCooldown),
				NextCheckAt: snapshot.Now.Add(time.Duration(remainingCooldown) * time.Second), Metrics: metrics,
			}, nil
		}
		arguments := nomadMinuteSkipArguments(target.Observation, settings.TimeSkipReserve)
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Apply a time skip to locked camp %d:%d", target.Observation.X, target.Observation.Y),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "nomad.cooldown.minute_skip", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if len(availableCommanders) == 0 {
		return nomadCommanderWaiting(snapshot.Now, commandersRestricted, metrics), nil
	}
	maximumLaunches := len(availableCommanders)
	if !settings.SkipCooldowns {
		maximumLaunches = 1
	} else {
		availableSkips := responseGatedDungeonCooldownCount(snapshot.State, settings.TimeSkipReserve, target.Definition.CooldownSec)
		committedSkips := int64(metrics["activeAttacks"])
		usableSkips := max(0, availableSkips-committedSkips)
		metrics["availableCooldownSkips"] = float64(availableSkips)
		metrics["committedCooldownSkips"] = float64(committedSkips)
		metrics["usableCooldownSkips"] = float64(usableSkips)
		if usableSkips < 1 {
			return Decision{
				Status: "waiting", Detail: "No confirmed cooldown skip is available above the configured reserve for another locked-camp attack",
				NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
			}, nil
		}
		maximumLaunches = min(maximumLaunches, int(usableSkips))
	}
	maximumLaunches = capDailyAttackBatch(maximumLaunches, dailyAllowance)
	launchCommanders, limitedPreset, err := availableNomadPresetCommanders(
		snapshot, source, target, preset, availableCommanders, maximumLaunches, true,
	)
	if err != nil {
		if decision, refresh := generalSkillsRefreshDecision(err, snapshot.Now, metrics); refresh {
			return decision, nil
		}
		return nomadWaiting(snapshot.Now, fmt.Sprintf("Cannot calculate %s inventory requirements: %v", preset.Name, err)), nil
	}
	metrics["presetCopies"] = float64(len(launchCommanders))
	if len(launchCommanders) == 0 {
		return nomadPresetWaiting(snapshot.Now, limitedPreset, source, snapshot.GameData, metrics), nil
	}
	metrics["chainSize"] = float64(len(launchCommanders))
	return nomadAttackDecision(snapshot.Now, score, settings, source, preset, target, launchCommanders, "chain", maximumVictoryCount, metrics), nil
}

func activeNomadEventScore(gameState State.GameState, now time.Time) (State.ScalableEventScore, bool) {
	if score, found := gameState.ActiveScalableEventScore(); found {
		if _, supported := nomadTargetType(score.EventID); supported && nomadScoreStillActive(score, now) {
			return score, true
		}
	}
	var selected State.ScalableEventScore
	found := false
	for _, eventID := range []int64{nomadEventID, samuraiEventID} {
		score, exists := gameState.LookupScalableEventScore(eventID)
		if !exists || !nomadScoreStillActive(score, now) {
			continue
		}
		if !found || score.ObservedAt.After(selected.ObservedAt) {
			selected, found = score, true
		}
	}
	return selected, found
}

func nomadScoreStillActive(score State.ScalableEventScore, now time.Time) bool {
	return score.RemainingSec > 0 && invasionEventRemaining(score, now) > 0
}

func configuredNomadDifficulty(settings autoNomadSettings, eventID int64) int64 {
	if eventID == nomadEventID {
		return settings.NomadDifficultyID
	}
	if eventID == samuraiEventID {
		return settings.SamuraiDifficultyID
	}
	return 0
}

func configuredNomadPresetID(settings autoNomadSettings, eventID int64) string {
	if eventID == nomadEventID {
		return settings.NomadPresetID
	}
	if eventID == samuraiEventID {
		return settings.SamuraiPresetID
	}
	return ""
}

func nomadTargetType(eventID int64) (int, bool) {
	if eventID == nomadEventID {
		return nomadCampTypeID, true
	}
	if eventID == samuraiEventID {
		return samuraiCampTypeID, true
	}
	return 0, false
}

func nomadEventName(eventID int64) string {
	if eventID == samuraiEventID {
		return "Samurai"
	}
	return "Nomad"
}

func nomadWaiting(now time.Time, detail string) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second)}
}

func nomadMapRefreshInterval(value int) time.Duration {
	if value < 60 {
		value = defaultNomadMapRefresh
	}
	return time.Duration(min(3600, value)) * time.Second
}

func nomadCampCandidates(
	gameState State.GameState,
	gameData *GameData.Store,
	source State.CastleState,
	eventID, difficultyID int64,
	targetTypeID int,
	lastScan time.Time,
) []nomadCampCandidate {
	result := make([]nomadCampCandidate, 0, nomadCampCount)
	gameState.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionEventCamp, func(_ string, observation State.MapObservation) bool {
		if observation.TypeID != targetTypeID || observation.ObservedAt.Before(lastScan) ||
			invasionDistanceSquared(source, observation) > fixedNomadRadius*fixedNomadRadius || observation.EventCampID <= 0 {
			return true
		}
		definition, found := gameData.EventCamp(observation.EventCampID)
		if !found || definition.EventID != eventID || definition.DifficultyID != difficultyID ||
			definition.AreaTypeID != targetTypeID || definition.VictoryCount != observation.EventCampVictoryCount {
			return true
		}
		result = append(result, nomadCampCandidate{Observation: observation, Definition: definition})
		return true
	})
	sort.Slice(result, func(left, right int) bool {
		leftDistance := invasionDistanceSquared(source, result[left].Observation)
		rightDistance := invasionDistanceSquared(source, result[right].Observation)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if result[left].Observation.Y != result[right].Observation.Y {
			return result[left].Observation.Y < result[right].Observation.Y
		}
		return result[left].Observation.X < result[right].Observation.X
	})
	return result
}

func pendingNomadCampRefresh(gameState State.GameState, camps []nomadCampCandidate) (State.NomadCampCooldownState, bool) {
	var selected State.NomadCampCooldownState
	found := false
	for _, camp := range camps {
		key := fmt.Sprintf("%d:%d:%d", camp.Observation.KingdomID, camp.Observation.X, camp.Observation.Y)
		cooldown, exists := gameState.NomadCamps.Cooldowns[key]
		if !exists || !cooldown.PendingCooldownRefresh {
			continue
		}
		if !found || cooldown.LastSuccessfulBattleAt.Before(selected.LastSuccessfulBattleAt) {
			selected, found = cooldown, true
		}
	}
	return selected, found
}

func lowestNomadCamp(camps []nomadCampCandidate, maximumVictoryCount int64) nomadCampCandidate {
	candidates := append([]nomadCampCandidate(nil), camps...)
	sort.SliceStable(candidates, func(left, right int) bool {
		leftCount := candidates[left].Observation.EventCampVictoryCount
		rightCount := candidates[right].Observation.EventCampVictoryCount
		if leftCount != rightCount {
			return leftCount < rightCount
		}
		return nomadCampDefenseScore(candidates[left]) < nomadCampDefenseScore(candidates[right])
	})
	for _, camp := range candidates {
		if camp.Observation.EventCampVictoryCount < maximumVictoryCount {
			return camp
		}
	}
	return candidates[0]
}

func weakestNomadCamp(camps []nomadCampCandidate) nomadCampCandidate {
	result := camps[0]
	for _, camp := range camps[1:] {
		left, right := nomadCampDefenseScore(camp), nomadCampDefenseScore(result)
		if left < right || left == right && (camp.Observation.Y < result.Observation.Y ||
			camp.Observation.Y == result.Observation.Y && camp.Observation.X < result.Observation.X) {
			result = camp
		}
	}
	return result
}

func nomadCampDefenseScore(camp nomadCampCandidate) int64 {
	observation := camp.Observation
	return camp.Definition.MaximumDefenseTroopCount*1000 + observation.EventCampBaseWallBonus +
		observation.EventCampBaseGateBonus + observation.EventCampBaseMoatBonus
}

func validNomadLock(
	locked *State.NomadCampTargetState,
	score State.ScalableEventScore,
	source State.CastleState,
	camps []nomadCampCandidate,
	maximumVictoryCount int64,
) bool {
	if locked == nil || locked.SourceCastleID != source.ID || locked.EventID != score.EventID ||
		locked.DifficultyID != score.DifficultyID {
		return false
	}
	currentEnd := nomadEventEndsAt(score)
	if !locked.EventEndsAt.IsZero() && !currentEnd.IsZero() {
		difference := locked.EventEndsAt.Sub(currentEnd)
		if difference < 0 {
			difference = -difference
		}
		if difference > 5*time.Minute {
			return false
		}
	}
	for _, camp := range camps {
		observation := camp.Observation
		if observation.KingdomID == locked.KingdomID && observation.X == locked.X && observation.Y == locked.Y &&
			observation.TypeID == locked.TypeID && observation.EventCampID == locked.EventCampID &&
			observation.EventCampVictoryCount >= maximumVictoryCount {
			return true
		}
	}
	return false
}

func lockedNomadCandidate(locked State.NomadCampTargetState, camps []nomadCampCandidate) (nomadCampCandidate, bool) {
	for _, camp := range camps {
		observation := camp.Observation
		if observation.KingdomID == locked.KingdomID && observation.X == locked.X && observation.Y == locked.Y &&
			observation.TypeID == locked.TypeID && observation.EventCampID == locked.EventCampID {
			return camp, true
		}
	}
	return nomadCampCandidate{}, false
}

func nomadEventEndsAt(score State.ScalableEventScore) time.Time {
	if score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		return time.Time{}
	}
	return score.ObservedAt.Add(time.Duration(score.RemainingSec) * time.Second)
}

func nomadCampCooldownRemaining(gameState State.GameState, observation State.MapObservation, now time.Time) int {
	remaining, observedAt := observation.EventCampCooldownRemaining, observation.ObservedAt
	key := fmt.Sprintf("%d:%d:%d", observation.KingdomID, observation.X, observation.Y)
	if cooldown, found := gameState.NomadCamps.Cooldowns[key]; found && cooldown.CooldownObservedAt.After(observedAt) {
		remaining, observedAt = cooldown.CooldownRemaining, cooldown.CooldownObservedAt
	}
	if remaining <= 0 {
		return 0
	}
	if !observedAt.IsZero() && now.After(observedAt) {
		remaining -= int(now.Sub(observedAt) / time.Second)
	}
	return max(0, remaining)
}

func activeNomadCampAttackCount(gameState State.GameState, sourceCastleID State.CastleID, camps []nomadCampCandidate, now time.Time) int {
	targets := map[string]struct{}{}
	for _, camp := range camps {
		targets[fmt.Sprintf("%d:%d:%d", camp.Observation.KingdomID, camp.Observation.X, camp.Observation.Y)] = struct{}{}
	}
	count := 0
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 0 || movement.SourceCastleID != sourceCastleID || !towerMovementActiveAt(movement, now) {
			return true
		}
		if _, found := targets[fmt.Sprintf("%d:%d:%d", movement.KingdomID, movement.TargetX, movement.TargetY)]; found {
			count++
		}
		return true
	})
	return count
}

func availableNomadCommanders(gameState State.GameState, candidates []State.CommanderID, restricted bool) []State.CommanderID {
	if !restricted {
		candidates = make([]State.CommanderID, 0, len(gameState.Commanders))
		for id := range gameState.Commanders {
			candidates = append(candidates, id)
		}
	}
	result := make([]State.CommanderID, 0, len(candidates))
	for _, id := range candidates {
		if commander, exists := gameState.Commanders[id]; exists && commander.Available {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func availablePresetCopies(
	preset AttackPresets.Preset,
	source State.CastleState,
	gameData *GameData.Store,
	maximum int,
) (int, error) {
	if maximum <= 0 {
		return 0, nil
	}
	_, hasLaunchTroops := AttackPresets.Requirements(preset)
	if !hasLaunchTroops {
		return 0, nil
	}
	return AttackPresets.MaximumAvailableCopies(preset, source.Units.Stationed, gameData, maximum)
}

func availableNomadPresetCommanders(
	snapshot Snapshot,
	source State.CastleState,
	target nomadCampCandidate,
	preset AttackPresets.Preset,
	commanderIDs []State.CommanderID,
	maximum int,
	aggregateInventory bool,
) ([]State.CommanderID, AttackPresets.Preset, error) {
	if maximum <= 0 || len(commanderIDs) == 0 {
		return nil, preset, nil
	}
	selected := make([]State.CommanderID, 0, min(maximum, len(commanderIDs)))
	committed := map[State.UnitID]int64{}
	var firstLimited AttackPresets.Preset
	var firstResolveErr error
	resolvedAny := false
	for _, commanderID := range commanderIDs {
		limited, err := autoNomadCapacityLimitedPreset(snapshot, source, target, preset, commanderID)
		if err != nil {
			if firstResolveErr == nil {
				firstResolveErr = err
			}
			continue
		}
		resolvedAny = true
		if firstLimited.ID == "" {
			firstLimited = limited
		}
		availableInventory := source.Units.Stationed
		if aggregateInventory && len(committed) > 0 {
			availableInventory = make(map[State.UnitID]int64, len(source.Units.Stationed))
			for itemID, amount := range source.Units.Stationed {
				availableInventory[itemID] = max(int64(0), amount-committed[itemID])
			}
		}
		resolved, shortage, resolveErr := AttackPresets.CheckInventory(
			limited, availableInventory, snapshot.GameData, 1,
		)
		if resolveErr != nil {
			if firstResolveErr == nil {
				firstResolveErr = resolveErr
			}
			continue
		}
		if shortage != nil {
			continue
		}
		required, hasLaunchTroops := AttackPresets.Requirements(resolved)
		if !hasLaunchTroops {
			continue
		}
		fits := true
		for itemID, amount := range required {
			committedAmount := int64(0)
			if aggregateInventory {
				committedAmount = committed[itemID]
			}
			if committedAmount+amount > source.Units.Stationed[itemID] {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		if aggregateInventory {
			for itemID, amount := range required {
				committed[itemID] += amount
			}
		}
		selected = append(selected, commanderID)
		if len(selected) >= maximum {
			break
		}
	}
	if !resolvedAny && firstResolveErr != nil {
		return nil, preset, firstResolveErr
	}
	if firstLimited.ID == "" {
		firstLimited = preset
	}
	return selected, firstLimited, nil
}

func autoNomadCapacityLimitedPreset(
	snapshot Snapshot,
	source State.CastleState,
	target nomadCampCandidate,
	preset AttackPresets.Preset,
	commanderID State.CommanderID,
) (AttackPresets.Preset, error) {
	dialog := snapshot.State.AttackDialog
	useAttackDialog := dialog.SourceCastleID == source.ID && dialog.KingdomID == target.Observation.KingdomID &&
		dialog.Target.TypeID == target.Observation.TypeID && dialog.Target.X == target.Observation.X &&
		dialog.Target.Y == target.Observation.Y && dialog.Target.EventCampID == target.Observation.EventCampID &&
		dialog.Target.EventCampVictoryCount == target.Observation.EventCampVictoryCount
	capacity, err := (AttackCapacity.Resolver{}).Resolve(snapshot.State, snapshot.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: useAttackDialog,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("event-camp:%d:%d:%d", target.Observation.KingdomID, target.Observation.X, target.Observation.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.Observation.KingdomID, TypeID: target.Observation.TypeID,
				X: target.Observation.X, Y: target.Observation.Y, ObjectID: target.Observation.EventCampID,
				Level: target.Definition.CampLevel, VictoryCount: target.Observation.EventCampVictoryCount,
			},
			Level: target.Definition.CampLevel, CastleTypeID: target.Observation.TypeID,
			PvP: false, LegendaryFight: false,
		},
	})
	if err != nil {
		return AttackPresets.Preset{}, fmt.Errorf("resolve Nomad/Samurai camp attack capacity: %w", err)
	}
	return AttackPresets.LimitToCapacity(preset, capacity), nil
}

func nomadCommanderWaiting(now time.Time, restricted bool, metrics map[string]float64) Decision {
	detail := "No commander is currently available"
	if restricted {
		detail = "No assigned Auto Nomad/Samurai commander is currently available"
	}
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second), Metrics: metrics}
}

func nomadPresetWaiting(
	now time.Time,
	preset AttackPresets.Preset,
	source State.CastleState,
	gameData *GameData.Store,
	metrics map[string]float64,
) Decision {
	if itemID, required, available, found, err := invasionPresetShortage(preset, source, gameData); err != nil {
		return Decision{
			Status: "waiting", Detail: "Cannot resolve preset troop families: " + err.Error(),
			NextCheckAt: now.Add(30 * time.Second), Metrics: metrics,
		}
	} else if found {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("Preset needs %d of item %d; source castle has %d", required, itemID, available),
			NextCheckAt: now.Add(30 * time.Second), Metrics: metrics,
		}
	}
	return Decision{Status: "waiting", Detail: "The selected preset has no launchable troops", NextCheckAt: now.Add(30 * time.Second), Metrics: metrics}
}

func nomadAttackDecision(
	now time.Time,
	score State.ScalableEventScore,
	settings autoNomadSettings,
	source State.CastleState,
	preset AttackPresets.Preset,
	target nomadCampCandidate,
	commanderIDs []State.CommanderID,
	mode string,
	maximumVictoryCount int64,
	metrics map[string]float64,
) Decision {
	arguments, _ := json.Marshal(map[string]any{
		"mode": mode, "sourceCastleId": source.ID, "eventId": score.EventID,
		"difficultyId": score.DifficultyID, "scoreTarget": settings.ScoreTarget,
		"minimumRemainingSec": settings.MinimumRemainingSec,
		"targetTypeId":        target.Observation.TypeID, "kingdomId": target.Observation.KingdomID,
		"targetX": target.Observation.X, "targetY": target.Observation.Y,
		"eventCampId":  target.Observation.EventCampID,
		"victoryCount": target.Observation.EventCampVictoryCount,
		"preset":       preset, "commanderIds": commanderIDs,
		"horseTravelBoostId": settings.HorseTravelBoostID, "dailyAttackLimit": settings.DailyAttackLimit,
	})
	detail := fmt.Sprintf("Level camp %d:%d from victory count %d/%d with %s", target.Observation.X, target.Observation.Y,
		target.Observation.EventCampVictoryCount, maximumVictoryCount, preset.Name)
	if mode == "chain" {
		detail = fmt.Sprintf("Chain %d attacks into locked camp %d:%d with %s", len(commanderIDs), target.Observation.X, target.Observation.Y, preset.Name)
	}
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "nomad.camp.attack", Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

func evaluateAutoNomadRBCTest(snapshot Snapshot, settings autoNomadSettings) (Decision, error) {
	trial := settings.RBCTest
	if settings.SourceCastleID <= 0 || settings.NomadPresetID == "" || trial.RunID == "" || trial.TargetX < 0 || trial.TargetY < 0 {
		return nomadWaiting(snapshot.Now, "RBC trial requires a source, Nomad preset, target, and run id"), nil
	}
	if !settings.SkipCooldowns {
		return nomadWaiting(snapshot.Now, "Enable cooldown time skips before starting the RBC trial"), nil
	}
	if invalidTimeSkipReserve(settings.TimeSkipReserve) {
		return nomadWaiting(snapshot.Now, "RBC trial time-skip reserves cannot be negative"), nil
	}
	if snapshot.GameData == nil {
		return nomadWaiting(snapshot.Now, "Official game data is unavailable"), nil
	}
	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists || source.KingdomID != 0 {
		return nomadWaiting(snapshot.Now, "RBC trial source must be an available Great Empire castle"), nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	preset, exists := AttackPresets.Find(document, settings.NomadPresetID)
	if !exists {
		return nomadWaiting(snapshot.Now, "The selected Nomad preset for the RBC trial no longer exists"), nil
	}
	target, exists := snapshot.State.LookupMapObservation(source.KingdomID, fmt.Sprintf("%d:%d", trial.TargetX, trial.TargetY))
	if !exists || target.TypeID != kingdomTowerMapTypeID {
		return nomadRBCTestMapRefresh(snapshot.Now, source.KingdomID, trial.TargetX, trial.TargetY, "Discover the configured RBC trial target", nil), nil
	}
	metrics := map[string]float64{"targetX": float64(target.X), "targetY": float64(target.Y)}
	dailyAllowance, blocked := dailyAttackLimitAllowance(
		snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics,
	)
	if blocked != nil {
		return *blocked, nil
	}
	test := snapshot.State.NomadCamps.RBCTest
	activeTest := test != nil && test.RunID == trial.RunID
	if !activeTest && (target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) >= 30*time.Second) {
		return nomadRBCTestMapRefresh(snapshot.Now, source.KingdomID, target.X, target.Y, "Refresh the RBC trial target before launch", metrics), nil
	}
	key := towerTargetKey(target.KingdomID, target.X, target.Y)
	if cooldown, found := snapshot.State.LookupTowerCooldown(key); found && cooldown.PendingCooldownRefresh &&
		(!activeTest || cooldown.LastSuccessfulBattleAt.After(test.StartedAt)) {
		return nomadRBCTestMapRefresh(snapshot.Now, target.KingdomID, target.X, target.Y,
			fmt.Sprintf("Refresh RBC %d:%d immediately after confirmed hit %d", target.X, target.Y, test.VictoriesConfirmed), metrics), nil
	}
	remaining := towerCooldownRemaining(target, snapshot.Now)
	metrics["cooldownRemaining"] = float64(remaining)
	if remaining > 0 {
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Clear RBC %d:%d cooldown before the next trial arrival", target.X, target.Y),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
			Request: &Intent.Request{
				Name: "nomad.cooldown.minute_skip", Arguments: nomadMinuteSkipArguments(target, settings.TimeSkipReserve),
			}, ReevaluateOnSuccess: true,
		}, nil
	}
	outstandingCooldownSkips := int64(0)
	if activeTest {
		metrics["expectedAttacks"] = float64(test.ExpectedAttacks)
		metrics["attacksLaunched"] = float64(test.AttacksLaunched)
		metrics["victoriesConfirmed"] = float64(test.VictoriesConfirmed)
		metrics["cooldownsSkipped"] = float64(test.CooldownsSkipped)
		if test.SafetyError != "" {
			return Decision{
				Status: "blocked", Detail: "RBC trial stopped on unsafe arrival order: " + test.SafetyError,
				NextCheckAt: snapshot.Now.Add(30 * time.Second), Metrics: metrics,
			}, nil
		}
		outstandingCooldownSkips = max(0, int64(test.AttacksLaunched-test.CooldownsSkipped))
	}
	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoNomad")
	available := availableNomadCommanders(snapshot.State, commanderIDs, restricted)
	availableSkips := responseGatedDungeonCooldownCount(snapshot.State, settings.TimeSkipReserve, 3*60*60)
	usableSkips := max(0, availableSkips-outstandingCooldownSkips)
	presetCopies, err := availablePresetCopies(preset, source, snapshot.GameData, len(available))
	if err != nil {
		return nomadWaiting(snapshot.Now, "Cannot resolve RBC preset troop families: "+err.Error()), nil
	}
	chainSize := min(len(available), presetCopies)
	if int64(chainSize) > usableSkips {
		chainSize = int(usableSkips)
	}
	chainSize = capDailyAttackBatch(chainSize, dailyAllowance)
	metrics["availableCommanders"] = float64(len(available))
	metrics["availableCooldownSkips"] = float64(availableSkips)
	metrics["committedCooldownSkips"] = float64(outstandingCooldownSkips)
	metrics["usableCooldownSkips"] = float64(usableSkips)
	metrics["presetCopies"] = float64(presetCopies)
	metrics["nextChainSize"] = float64(chainSize)
	if chainSize < 1 {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf(
				"No resource-backed RBC attack is ready now (%d commanders, %d complete preset copies, %d usable cooldown skips, %d committed to launched hits)",
				len(available), presetCopies, usableSkips, outstandingCooldownSkips,
			),
			NextCheckAt: snapshot.Now.Add(30 * time.Second), Metrics: metrics,
		}, nil
	}
	available = available[:chainSize]
	batchID := fmt.Sprintf("%s-%d", trial.RunID, snapshot.Now.UnixNano())
	arguments, _ := json.Marshal(map[string]any{
		"runId": trial.RunID, "batchId": batchID, "sourceCastleId": source.ID, "kingdomId": target.KingdomID,
		"targetX": target.X, "targetY": target.Y, "victoryCount": target.TowerVictoryCount,
		"expectedAttacks": chainSize,
		"preset":          preset, "commanderIds": available,
		"horseTravelBoostId": settings.HorseTravelBoostID, "dailyAttackLimit": settings.DailyAttackLimit,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch resource-sized %d-hit Auto Camp trial at RBC %d:%d", chainSize, target.X, target.Y),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "nomad.rbc_test.attack", Arguments: arguments}, ReevaluateOnSuccess: true,
	}, nil
}

func nomadRBCTestMapRefresh(
	now time.Time,
	kingdomID State.KingdomID,
	x, y int,
	detail string,
	metrics map[string]float64,
) Decision {
	arguments, _ := json.Marshal(map[string]any{"kingdomId": kingdomID, "x1": x, "y1": y, "x2": x, "y2": y})
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

func nomadMinuteSkipArguments(observation State.MapObservation, reserves map[string]int64) json.RawMessage {
	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": observation.KingdomID, "targetTypeId": observation.TypeID,
		"targetX": observation.X, "targetY": observation.Y, "eventCampId": observation.EventCampID,
		"minimumRemaining": reserves,
	})
	return arguments
}

func oneCommandDungeonSkipCount(gameState State.GameState, reserves map[string]int64, cooldownSec int64) int64 {
	options := []struct {
		wireKey  string
		currency State.CurrencyID
		minutes  int64
	}{
		{"MS1", 1001, 1}, {"MS2", 1002, 5}, {"MS3", 1003, 10}, {"MS4", 1004, 30},
		{"MS5", 1005, 60}, {"MS6", 1006, 300}, {"MS7", 1007, 1440},
	}
	var count int64
	for _, option := range options {
		if option.minutes*60 < cooldownSec {
			continue
		}
		available := int64(gameState.Player.Currencies[option.currency]) - automationTimeSkipReserve(reserves, option.wireKey)
		if available > 0 {
			count += available
		}
	}
	return count
}

func responseGatedDungeonCooldownCount(gameState State.GameState, reserves map[string]int64, cooldownSec int64) int64 {
	if cooldownSec <= 0 {
		return 0
	}
	options := []struct {
		wireKey  string
		currency State.CurrencyID
		seconds  int64
	}{
		{"MS1", 1001, 60}, {"MS2", 1002, 300}, {"MS3", 1003, 600}, {"MS4", 1004, 1800},
		{"MS5", 1005, 3600}, {"MS6", 1006, 18000}, {"MS7", 1007, 86400},
	}
	available := make([]int64, len(options))
	totalSeconds := float64(0)
	for index, option := range options {
		available[index] = max(
			int64(0),
			int64(gameState.Player.Currencies[option.currency])-automationTimeSkipReserve(reserves, option.wireKey),
		)
		totalSeconds += float64(available[index]) * float64(option.seconds)
	}
	maximumCooldowns := min(int64(totalSeconds/float64(cooldownSec)), int64(10_000))
	var count int64
	for count < maximumCooldowns {
		remaining := cooldownSec
		for remaining > 0 {
			selected := -1
			for index, option := range options {
				if available[index] > 0 && option.seconds >= remaining {
					selected = index
					break
				}
			}
			if selected < 0 {
				for index := len(options) - 1; index >= 0; index-- {
					if available[index] > 0 {
						selected = index
						break
					}
				}
			}
			if selected < 0 {
				return count
			}
			available[selected]--
			remaining -= options[selected].seconds
		}
		count++
	}
	return count
}

func automationTimeSkipReserve(reserves map[string]int64, wireKey string) int64 {
	for key, value := range reserves {
		if strings.EqualFold(strings.TrimSpace(key), wireKey) {
			return value
		}
	}
	return 0
}

func invalidTimeSkipReserve(reserves map[string]int64) bool {
	for _, value := range reserves {
		if value < 0 {
			return true
		}
	}
	return false
}
