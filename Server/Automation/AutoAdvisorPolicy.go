package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoAdvisorSection         = "automation.autoAdvisor"
	autoAdvisorMaxAttackCount  = 9999
	autoAdvisorDefaultCoinCost = int64(500)
)

type AutoAdvisorPolicy struct{}

type autoAdvisorSettings struct {
	Version               int              `json:"version"`
	SourceCastleID        State.CastleID   `json:"sourceCastleId"`
	PresetID              string           `json:"presetId"`
	NomadDifficultyID     int64            `json:"nomadDifficultyId"`
	SamuraiDifficultyID   int64            `json:"samuraiDifficultyId"`
	MaxAttackCount        int              `json:"maxAttackCount"`
	MinimumRemainingSec   int64            `json:"minimumRemainingSec"`
	CoinCostPerAttack     int64            `json:"coinCostPerAttack"`
	MinimumCoinReserve    int64            `json:"minimumCoinReserve"`
	RubyCostPerAttack     int64            `json:"rubyCostPerAttack"`
	MinimumRubyReserve    int64            `json:"minimumRubyReserve"`
	MinimumFeatherReserve int64            `json:"minimumFeatherReserve"`
	TimeSkipReserve       map[string]int64 `json:"timeSkipReserve"`
	CheckIntervalSec      int              `json:"checkIntervalSec"`
	MapRefreshIntervalSec int              `json:"mapRefreshIntervalSec"`
	HorseTravelBoostID    int              `json:"horseTravelBoostId"`
}

func NewAutoAdvisorPolicy() *AutoAdvisorPolicy { return &AutoAdvisorPolicy{} }

func (*AutoAdvisorPolicy) ID() string { return "autoAdvisor" }

func (*AutoAdvisorPolicy) EnabledKey() string { return "auto_advisor" }

func (*AutoAdvisorPolicy) WakeDomains() []string {
	return []string{
		"advisor", "map-event-camp", "commanders", "units", "events", "event-scores", "nomad-camps",
		"currencies", "resources", "attack_dialog", "achievements",
	}
}

func (*AutoAdvisorPolicy) WakeSections() []string {
	return []string{autoAdvisorSection, AttackPresets.ConfigurationSection, commanderFeatureSection}
}

func (*AutoAdvisorPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := autoAdvisorSettings{
		MaxAttackCount:      autoAdvisorMaxAttackCount,
		MinimumRemainingSec: 1800, CoinCostPerAttack: autoAdvisorDefaultCoinCost,
		TimeSkipReserve: map[string]int64{}, CheckIntervalSec: 30,
		MapRefreshIntervalSec: defaultNomadMapRefresh, HorseTravelBoostID: -1,
	}
	if !decodeSection(snapshot.Configuration, autoAdvisorSection, &settings) {
		return autoAdvisorWaiting(snapshot.Now, "Auto Advisor is not configured", nil), nil
	}
	settings.PresetID = strings.TrimSpace(settings.PresetID)
	if settings.SourceCastleID <= 0 || settings.PresetID == "" || settings.NomadDifficultyID <= 0 || settings.SamuraiDifficultyID <= 0 {
		return autoAdvisorWaiting(snapshot.Now, "Choose a source castle, attack preset, and both event difficulties", nil), nil
	}
	if settings.MaxAttackCount < 1 || settings.MaxAttackCount > autoAdvisorMaxAttackCount {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf("Maximum attacks must be between 1 and %d", autoAdvisorMaxAttackCount), nil), nil
	}
	if settings.MinimumRemainingSec < 0 || settings.CoinCostPerAttack <= 0 || settings.MinimumCoinReserve < 0 || settings.RubyCostPerAttack < 0 ||
		settings.MinimumRubyReserve < 0 || settings.MinimumFeatherReserve < 0 ||
		invalidTimeSkipReserve(settings.TimeSkipReserve) || !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return autoAdvisorWaiting(snapshot.Now, "Advisor timing, travel, cost, or reserve settings are invalid", nil), nil
	}
	if autoAdvisorUsesRubyHorse(settings.HorseTravelBoostID) && settings.RubyCostPerAttack <= 0 {
		return autoAdvisorWaiting(snapshot.Now, "Set a positive ruby cost per attack before using a ruby horse", nil), nil
	}
	score, found := activeNomadEventScore(snapshot.State, snapshot.Now)
	if !found {
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{nomadEventID, samuraiEventID}, "Nomad or Samurai event",
		); locked {
			return decision, nil
		}
		return autoAdvisorWaiting(snapshot.Now, "No Nomad or Samurai event is active", nil), nil
	}
	targetTypeID, _ := nomadTargetType(score.EventID)
	difficultyID := autoAdvisorDifficulty(settings, score.EventID)
	metrics := map[string]float64{
		"eventId": float64(score.EventID), "score": float64(score.PlayerScore),
		"advisorActive": boolMetric(score.AdvisorActive),
	}
	if snapshot.GameData == nil {
		return autoAdvisorWaiting(snapshot.Now, "Official event data is unavailable", metrics), nil
	}
	difficulty, valid := snapshot.GameData.ScalableEvent(score.EventID, difficultyID)
	if !valid {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not valid for event %d", difficultyID, score.EventID), metrics), nil
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !snapshot.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not unlocked", difficultyID), metrics), nil
	}
	if score.DifficultyID <= 0 {
		arguments, _ := json.Marshal(map[string]any{"eventId": score.EventID, "difficultyId": difficultyID})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Start %s at difficulty %d before activating the advisor", nomadEventName(score.EventID), difficultyID),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "nomad.difficulty.select", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if score.DifficultyID != difficultyID {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf(
			"The active %s run uses difficulty %d; configured difficulty %d applies to the next event",
			nomadEventName(score.EventID), score.DifficultyID, difficultyID,
		), metrics), nil
	}
	if !score.AdvisorActive {
		tokenID := score.AdvisorCurrencyID
		if tokenID == 0 {
			tokenID = autoAdvisorEventTokenID(score.EventID)
		}
		metrics["advisorTokenCurrencyId"] = float64(tokenID)
		metrics["advisorEventTokens"] = snapshot.State.Player.Currencies[tokenID]
		metrics["advisorUniversalTokens"] = snapshot.State.Player.Currencies[76]
		return autoAdvisorWaiting(snapshot.Now, "Advisor is locked; activation requires explicit token confirmation in settings", metrics), nil
	}
	if run := snapshot.State.Advisor.Run; run != nil && autoAdvisorRunMatchesEvent(*run, score) {
		metrics["requestedAttacks"] = float64(run.RequestedAttacks)
		metrics["currentAttack"] = float64(run.CurrentAttack)
		metrics["remainingAttacks"] = float64(max(0, run.RequestedAttacks-run.CurrentAttack))
		if run.Status == "running" {
			if snapshot.State.Advisor.Summary.ObservedAt.IsZero() || snapshot.Now.Sub(snapshot.State.Advisor.Summary.ObservedAt) >= time.Minute {
				return Decision{
					Status: "running", Detail: fmt.Sprintf("Advisor attack %d of %d is in progress", run.CurrentAttack, run.RequestedAttacks),
					NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
					Request: &Intent.Request{Name: "advisor.overview.refresh", Arguments: json.RawMessage(`{}`)},
				}, nil
			}
			return Decision{
				Status: "running", Detail: fmt.Sprintf("Advisor attack %d of %d is in progress", run.CurrentAttack, run.RequestedAttacks),
				NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
			}, nil
		}
		if run.Status == "completed" {
			return Decision{
				Status: "complete", Detail: fmt.Sprintf("Advisor completed all %d requested attacks", run.RequestedAttacks),
				NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
			}, nil
		}
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Advisor run was %s after attack %d; it will not be restarted automatically", run.Status, run.CurrentAttack),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	}

	remaining := invasionEventRemaining(score, snapshot.Now)
	usableSeconds := remaining - settings.MinimumRemainingSec
	if usableSeconds < State.AdvisorEstimatedCycleSeconds {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf("Event has only %d usable seconds remaining", max(int64(0), usableSeconds)), metrics), nil
	}
	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists || source.KingdomID != 0 {
		return autoAdvisorWaiting(snapshot.Now, "Advisor attacks require the configured Great Empire source castle", metrics), nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	preset, exists := AttackPresets.Find(document, settings.PresetID)
	if !exists {
		return autoAdvisorWaiting(snapshot.Now, "The selected CitadelOps attack preset no longer exists", metrics), nil
	}
	refreshInterval := nomadMapRefreshInterval(settings.MapRefreshIntervalSec)
	lastScan := snapshot.State.NomadCamps.LastScannedAt[source.ID]
	if lastScan.IsZero() || snapshot.Now.Sub(lastScan) >= refreshInterval {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "radius": fixedNomadRadius, "scanStartedAt": snapshot.Now,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Discover the four %s camps for Advisor targeting", nomadEventName(score.EventID)),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "nomad.map.scan", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	camps := nomadCampCandidates(snapshot.State, snapshot.GameData, source, score.EventID, score.DifficultyID, targetTypeID, lastScan)
	metrics["knownCamps"] = float64(len(camps))
	if len(camps) < nomadCampCount {
		return autoAdvisorWaiting(snapshot.Now, fmt.Sprintf("Found %d of the expected %d regular camps", len(camps), nomadCampCount), metrics), nil
	}
	camps = camps[:nomadCampCount]
	target := weakestNomadCamp(camps)
	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoAdvisor")
	availableCommanders := availableNomadCommanders(snapshot.State, commanderIDs, restricted)
	if len(availableCommanders) == 0 {
		detail := "No commander is currently available"
		if restricted {
			detail = "No assigned Auto Advisor commander is currently available"
		}
		return autoAdvisorWaiting(snapshot.Now, detail, metrics), nil
	}

	eventCapacity := min(autoAdvisorMaxAttackCount, int(usableSeconds/State.AdvisorEstimatedCycleSeconds))
	inventoryCapacity := availablePresetCopies(preset, source, autoAdvisorMaxAttackCount)
	coinCapacity := int(math.Floor(max(0, playerResourceAmount(snapshot, "C1")-float64(settings.MinimumCoinReserve)) / float64(settings.CoinCostPerAttack)))
	rubyCapacity := autoAdvisorMaxAttackCount
	if autoAdvisorUsesRubyHorse(settings.HorseTravelBoostID) {
		rubyCapacity = int(math.Floor(max(0, playerResourceAmount(snapshot, "C2")-float64(settings.MinimumRubyReserve)) / float64(settings.RubyCostPerAttack)))
	}
	featherCapacity := autoAdvisorMaxAttackCount
	if settings.HorseTravelBoostID <= 0 {
		featherID := currencyIDForJSONKey(snapshot.GameData, "PTT")
		featherCapacity = max(0, int(snapshot.State.Player.Currencies[featherID])-int(settings.MinimumFeatherReserve))
	}
	timeSkipCapacity := autoAdvisorMaxAttackCount
	if target.Definition.CooldownSec > 0 {
		timeSkipCapacity = min(autoAdvisorMaxAttackCount, 1+int(oneCommandDungeonSkipCount(
			snapshot.State, settings.TimeSkipReserve, int64(target.Definition.CooldownSec),
		)))
	}
	attackCount := min(settings.MaxAttackCount, eventCapacity, inventoryCapacity, coinCapacity, rubyCapacity, featherCapacity, timeSkipCapacity)
	metrics["eventCapacity"] = float64(eventCapacity)
	metrics["inventoryCapacity"] = float64(inventoryCapacity)
	metrics["coinCapacity"] = float64(coinCapacity)
	metrics["rubyCapacity"] = float64(rubyCapacity)
	metrics["featherCapacity"] = float64(featherCapacity)
	metrics["timeSkipCapacity"] = float64(timeSkipCapacity)
	metrics["plannedAttacks"] = float64(max(0, attackCount))
	if attackCount < 1 {
		return autoAdvisorWaiting(snapshot.Now, "No advisor attack fits all event-time, troop/tool, coin, ruby, feather, and time-skip reserves", metrics), nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": source.ID, "eventId": score.EventID, "difficultyId": score.DifficultyID,
		"kingdomId": target.Observation.KingdomID, "targetTypeId": target.Observation.TypeID,
		"targetX": target.Observation.X, "targetY": target.Observation.Y, "eventCampId": target.Observation.EventCampID,
		"preset": preset, "commanderId": availableCommanders[0], "attackCount": attackCount,
		"minimumRemainingSec": settings.MinimumRemainingSec,
		"coinCostPerAttack":   settings.CoinCostPerAttack, "minimumCoinReserve": settings.MinimumCoinReserve,
		"rubyCostPerAttack": settings.RubyCostPerAttack, "minimumRubyReserve": settings.MinimumRubyReserve,
		"minimumFeatherReserve": settings.MinimumFeatherReserve, "timeSkipReserve": settings.TimeSkipReserve,
		"horseTravelBoostId": settings.HorseTravelBoostID,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch %d advisor attacks against %s camp %d:%d", attackCount, nomadEventName(score.EventID), target.Observation.X, target.Observation.Y),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "advisor.run.launch", Arguments: arguments}, ReevaluateOnSuccess: true,
	}, nil
}

func autoAdvisorWaiting(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second), Metrics: metrics}
}

func autoAdvisorDifficulty(settings autoAdvisorSettings, eventID int64) int64 {
	if eventID == nomadEventID {
		return settings.NomadDifficultyID
	}
	if eventID == samuraiEventID {
		return settings.SamuraiDifficultyID
	}
	return 0
}

func autoAdvisorEventTokenID(eventID int64) State.CurrencyID {
	if eventID == nomadEventID {
		return 77
	}
	if eventID == samuraiEventID {
		return 78
	}
	return 0
}

func autoAdvisorUsesRubyHorse(horseTravelBoostID int) bool {
	return horseTravelBoostID == 1008 || horseTravelBoostID == 1009
}

func autoAdvisorRunMatchesEvent(run State.AdvisorRunState, score State.ScalableEventScore) bool {
	if run.EventID != score.EventID {
		return false
	}
	currentEnd := nomadEventEndsAt(score)
	if run.EventEndsAt.IsZero() || currentEnd.IsZero() {
		return run.Status == "running" || !run.UpdatedAt.IsZero() && run.UpdatedAt.After(score.ObservedAt.Add(-time.Minute))
	}
	delta := run.EventEndsAt.Sub(currentEnd)
	return delta >= -10*time.Minute && delta <= 10*time.Minute
}

func boolMetric(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
