package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	hospitalHealingBase         int64 = 5
	hospitalHealingMaximum      int64 = 15
	hospitalHealingEffectID     int64 = 103
	hospitalHealingEffectTypeID int64 = 106
	hospitalEvidenceMaxAge            = 30 * time.Second
)

// resolveHospitalHealStep runs after committed JAA, REI and SIE responses. The
// requested amount is an upper bound, including for manually submitted intents.
func resolveHospitalHealStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
		UnitID   State.UnitID   `json:"unitId"`
		Amount   int64          `json:"amount"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	state := input.State
	castle, ok := state.Castles[request.CastleID]
	if !ok || !castle.Focused || State.CastleFocusKnownUnavailable(state, castle) ||
		(input.ProtocolContext.FocusedCastleID > 0 && input.ProtocolContext.FocusedCastleID != request.CastleID) {
		return Intent.Step{}, fmt.Errorf("%w: hospital castle focus changed", Intent.ErrPlanStale)
	}
	now := time.Now().UTC()
	snapshotAt := castle.ContextSnapshotObservedAt
	queue, observed := castle.Production[2]
	if snapshotAt.IsZero() || now.Sub(snapshotAt) > hospitalEvidenceMaxAge ||
		castle.UnitsObservedAt.Before(snapshotAt) || !observed || queue.ObservedAt.Before(snapshotAt) ||
		queue.Capacity <= 0 || hospitalOccupiedSlots(queue) >= queue.Capacity {
		return Intent.Step{}, fmt.Errorf("%w: hospital wounds or queue slots need a fresh castle snapshot", Intent.ErrPlanStale)
	}
	wounded := castle.Units.Hospital[request.UnitID]
	if request.Amount <= 0 || wounded <= 0 {
		return Intent.Step{}, fmt.Errorf("%w: wounded count changed", Intent.ErrPlanStale)
	}
	// A failed or malformed entitlement refresh can never increase the base
	// allowance. Each bonus is independently proven by a fresh response.
	bonus := hospitalResearchBonus(state, input.GameData, snapshotAt, now) + hospitalSubscriptionBonus(state, input.GameData, snapshotAt, now)
	allowed := hospitalHealingBase + bonus
	if allowed > hospitalHealingMaximum {
		allowed = hospitalHealingMaximum
	}
	amount := request.Amount
	if amount > wounded {
		amount = wounded
	}
	if amount > allowed {
		amount = allowed
	}
	payload, _ := json.Marshal(map[string]any{"U": request.UnitID, "A": amount})
	step := commandStep("Heal wounded units", "hru", payload, "hru")
	step.StaleCodes = []int{175}
	guardArguments, _ := json.Marshal(map[string]any{
		"castleId": request.CastleID, "unitId": request.UnitID, "amount": amount,
	})
	step.FinalDispatchAction = "hospital.heal.guard"
	step.FinalDispatchArguments = guardArguments
	return step, nil
}

func (application *Application) guardHospitalHealDispatch(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("hospital heal state is unavailable")
	}
	store, ready := application.GameData.Current()
	if !ready || store == nil {
		return fmt.Errorf("official hospital catalog is unavailable")
	}
	state := application.State.ReadOnlyView()
	step, err := resolveHospitalHealStep(ctx, Intent.PlanningContext{State: state, GameData: store}, arguments)
	if err != nil {
		return err
	}
	var requested struct {
		Amount int64 `json:"amount"`
	}
	var allowed struct {
		Amount int64 `json:"A"`
	}
	if err := json.Unmarshal(arguments, &requested); err != nil {
		return err
	}
	if err := json.Unmarshal(step.Command.Payload, &allowed); err != nil {
		return err
	}
	if allowed.Amount != requested.Amount {
		return fmt.Errorf("%w: hospital heal entitlement changed before dispatch", Intent.ErrPlanStale)
	}
	return nil
}

func hospitalOccupiedSlots(queue State.ProductionQueue) int {
	occupied := len(queue.Queued)
	if queue.Active != nil {
		occupied++
	}
	return occupied
}

func hospitalEffectVerified(store *GameData.Store) bool {
	if store == nil {
		return false
	}
	catalog, err := store.Catalog("effects")
	if err != nil {
		return false
	}
	raw, found := catalog.Find(strconv.FormatInt(hospitalHealingEffectID, 10))
	if !found {
		return false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return false
	}
	effectType, ok := record.Int64("effectTypeID")
	name, named := record.String("name")
	return ok && effectType == hospitalHealingEffectTypeID && named && name == "hospitalSlotBonus"
}

func hospitalResearchBonus(state State.GameState, store *GameData.Store, after, now time.Time) int64 {
	observation := state.Research
	if observation.ObservedAt.Before(after) || observation.ObservedAt.IsZero() ||
		now.Sub(observation.ObservedAt) > hospitalEvidenceMaxAge || observation.Generation != state.Session.Generation ||
		!hospitalEffectVerified(store) {
		return 0
	}
	catalog, err := store.Catalog("researches")
	if err != nil {
		return 0
	}
	var bonus int64
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return 0
		}
		id, ok := hospitalResearchID(record)
		if !ok {
			return 0
		}
		if !observation.CompletedIDs[id] {
			continue
		}
		effects, ok := record.String("effects")
		if !ok {
			continue
		}
		value, valid := hospitalEffectValue(effects)
		if !valid {
			return 0
		}
		bonus += value
	}
	if bonus < 0 || bonus > hospitalHealingMaximum-hospitalHealingBase {
		return 0
	}
	return bonus
}

func hospitalResearchID(record GameData.Record) (int64, bool) {
	for _, field := range []string{"researchID", "researchId", "id", "ID"} {
		if id, ok := record.Int64(field); ok && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func hospitalSubscriptionBonus(state State.GameState, store *GameData.Store, after, now time.Time) int64 {
	observedAt := state.SubscriptionsObservedAt
	if observedAt.IsZero() || observedAt.Before(after) || now.Sub(observedAt) > hospitalEvidenceMaxAge ||
		state.SubscriptionsGeneration != state.Session.Generation || !hospitalEffectVerified(store) {
		return 0
	}
	var bonus int64
	for typeID, subscription := range state.Subscriptions {
		if typeID <= 0 || subscription.TypeID != typeID || subscription.RemainingSec <= 0 ||
			observedAt.Add(time.Duration(subscription.RemainingSec)*time.Second).Before(now) {
			continue
		}
		for _, effect := range store.SubscriptionEffectsView(typeID) {
			if int64(effect.ID) == hospitalHealingEffectID && effect.Value > 0 {
				bonus += effect.Value
			}
		}
	}
	if bonus < 0 || bonus > hospitalHealingMaximum-hospitalHealingBase {
		return 0
	}
	return bonus
}

func hospitalEffectValue(encoded string) (int64, bool) {
	var bonus int64
	for _, part := range strings.Split(encoded, ",") {
		idText, valueText, ok := strings.Cut(strings.TrimSpace(part), "&")
		if !ok {
			return 0, false
		}
		id, err := strconv.ParseInt(strings.TrimSpace(idText), 10, 64)
		if err != nil {
			return 0, false
		}
		value, err := strconv.ParseInt(strings.TrimSpace(valueText), 10, 64)
		if err != nil {
			return 0, false
		}
		if id == hospitalHealingEffectID {
			if value < 0 || value > hospitalHealingMaximum-hospitalHealingBase {
				return 0, false
			}
			bonus += value
		}
	}
	return bonus, true
}
