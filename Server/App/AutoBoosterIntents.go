package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	autoBoosterPurchaseCursorKey   = "global-effect/2"
	autoBoosterPurchaseFreshness   = 2 * time.Minute
	autoBoosterPurchaseWindowGuard = 30 * time.Second
)

type autoBoosterPurchaseRequest struct {
	GlobalEffectID             int64     `json:"globalEffectId"`
	ExpectedEndsAtUnix         int64     `json:"expectedEndsAtUnix"`
	ExpectedRubyCost           int64     `json:"expectedRubyCost"`
	ExpectedBonusValue         int64     `json:"expectedBonusValue"`
	MinimumRubyReserve         int64     `json:"minimumRubyReserve"`
	ExpectedCheckIntervalSec   int       `json:"expectedCheckIntervalSec"`
	ExpectedRubyBalance        int64     `json:"expectedRubyBalance"`
	ExpectedBaselineObservedAt time.Time `json:"expectedBaselineObservedAt,omitempty"`
	ExpectedRubyObservedAt     time.Time `json:"expectedRubyObservedAt,omitempty"`
	ExpectedSessionGeneration  uint64    `json:"expectedSessionGeneration,omitempty"`
	ExpectedRubyResourceID     int64     `json:"expectedRubyResourceId,omitempty"`
}

func (application *Application) registerAutoBoosterIntents() error {
	definitions := []Intent.Definition{
		{Name: "autoBooster.refresh", Description: "Refresh the authoritative account baseline for the daily global boost", DescriptionDescriptor: Localization.New("server.intent.description.1adb387e", "Refresh the authoritative account baseline for the daily global boost", nil), Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{}`), Planner: planAutoBoosterRefresh},
		{Name: "autoBooster.purchase", Description: "Purchase only the live server-quoted daily fortress-speed global boost", DescriptionDescriptor: Localization.New("server.intent.description.e67cd812", "Purchase only the live server-quoted daily fortress-speed global boost", nil), Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"globalEffectId":2,"expectedEndsAtUnix":1788382800,"expectedRubyCost":2500,"expectedBonusValue":50,"minimumRubyReserve":0,"expectedCheckIntervalSec":60,"expectedRubyBalance":10000}`), Planner: planAutoBoosterPurchase},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	for name, action := range map[string]Intent.Action{
		"auto_booster.purchase.arm":       application.armAutoBoosterPurchase,
		"auto_booster.purchase.dispatch":  application.finalizeAutoBoosterDispatch,
		"auto_booster.purchase.disarm":    application.disarmAutoBoosterPurchase,
		"auto_booster.purchase.reject":    application.rejectAutoBoosterPurchase,
		"auto_booster.purchase.reconcile": application.reconcileAutoBoosterPurchase,
	} {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	return nil
}

func autoBoosterGBDStep(name string) Intent.Step {
	step := commandStep(name, "gbd", nil, "gbd")
	step.Command = Protocol.Command{Opcode: "gbd", Bare: true}
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	step.CaptureResponse = false
	return step
}

func planAutoBoosterRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{Claims: []string{"shop", "events", "global-effect:2", "account-resources"}, Summary: "Refresh the daily fortress-speed boost account snapshot", SummaryDescriptor: Localization.New("server.app.refresh_the_daily_fortress.6eb6d185", "Refresh the daily fortress-speed boost account snapshot", nil), Steps: []Intent.Step{
		autoBoosterGBDStep("Refresh daily global-effect account snapshot"),
		{Name: "Reconcile unresolved boost purchase", Action: "auto_booster.purchase.reconcile"},
	}}, nil
}

func planAutoBoosterPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := autoBoosterPurchaseContext(input, arguments, time.Now().UTC(), true)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	payload, _ := json.Marshal(map[string]any{"GEID": request.GlobalEffectID})
	purchase := shopCommandStep("Activate daily fortress-speed boost", "agb", payload, 0)
	purchase.ResponseBarrier = Intent.ResponseBarrierCommitted
	purchase.CaptureResponse = true
	purchase.PreDispatchAction, purchase.PreDispatchArguments = "auto_booster.purchase.arm", resolved
	purchase.FinalDispatchAction, purchase.FinalDispatchArguments = "auto_booster.purchase.dispatch", resolved
	purchase.DefinitiveSendFailureAction, purchase.DefinitiveSendFailureArguments = "auto_booster.purchase.disarm", resolved
	purchase.DefinitiveResponseFailureAction, purchase.DefinitiveResponseFailureArguments = "auto_booster.purchase.reject", resolved
	purchase.ResponseProjectionFailureIndeterminate = true
	return Intent.Plan{
		Claims:  []string{"shop", "events", "global-effect:" + strconv.FormatInt(request.GlobalEffectID, 10), "account-resources"},
		Summary: fmt.Sprintf("Activate the daily fortress-speed boost for %d rubies", request.ExpectedRubyCost), SummaryDescriptor: Localization.New("server.app.activate_the_daily_fortress.3bc78a92", "Activate the daily fortress-speed boost for {p0} rubies", Localization.Params{"p0": request.ExpectedRubyCost}),
		Steps: []Intent.Step{
			autoBoosterGBDStep("Refresh account snapshot before boost purchase"), purchase,
			autoBoosterGBDStep("Refresh account snapshot after boost purchase"),
			{Name: "Reconcile boost purchase evidence", Action: "auto_booster.purchase.reconcile"},
		},
	}, nil
}

func (application *Application) autoBoosterPlanningContext() (Intent.PlanningContext, error) {
	if application == nil || application.State == nil || application.GameData == nil {
		return Intent.PlanningContext{}, Localization.WithError(fmt.Errorf("Auto Booster state is unavailable"), Localization.New("server.app.auto_booster_state_is.74c02e9a", "Auto Booster state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Intent.PlanningContext{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	return Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: gameData}, nil
}

func autoBoosterPurchaseContext(input Intent.PlanningContext, arguments json.RawMessage, now time.Time, requireFresh bool) (autoBoosterPurchaseRequest, error) {
	return autoBoosterPurchaseContextForOperation(input, arguments, now, requireFresh, "")
}

func autoBoosterPurchaseContextForOperation(input Intent.PlanningContext, arguments json.RawMessage, now time.Time, requireFresh bool, pendingOperationID string) (autoBoosterPurchaseRequest, error) {
	var request autoBoosterPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, err
	}
	if request.GlobalEffectID != GameData.FortressDailyGlobalEffectID || request.ExpectedRubyCost != GameData.FortressDailyBoosterRubyCost || request.ExpectedBonusValue <= 0 || request.ExpectedEndsAtUnix <= 0 || request.MinimumRubyReserve < 0 || request.ExpectedCheckIntervalSec < 30 || request.ExpectedCheckIntervalSec > 3600 || request.ExpectedRubyBalance < 0 {
		return request, Localization.WithError(fmt.Errorf("Auto Booster request does not match the approved 2,500-ruby fortress-speed purchase"), Localization.New("server.app.auto_booster_request_does.3412eae3", "Auto Booster request does not match the approved 2,500-ruby fortress-speed purchase", nil))
	}
	if input.GameData == nil {
		return request, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	contract, err := input.GameData.FortressSpeed()
	if err != nil {
		return request, err
	}
	if contract.DailyGlobalEffectID != request.GlobalEffectID || contract.DailyBoostPercent <= 0 {
		return request, Localization.WithError(fmt.Errorf("official fortress-speed global effect changed; refusing purchase"), Localization.New("server.app.official_fortress_speed_global.57f52413", "official fortress-speed global effect changed; refusing purchase", nil))
	}
	inventory := input.State.EventScores.Inventory
	baseline := inventory.GlobalEffectBaselineObservedAt
	if baseline.IsZero() || baseline.After(now) || inventory.GlobalEffectBaselineGeneration != input.State.Session.ConnectionGeneration || (!input.State.Session.ChangedAt.IsZero() && baseline.Before(input.State.Session.ChangedAt)) || requireFresh && now.Sub(baseline) >= autoBoosterPurchaseFreshness {
		return request, Localization.WithError(fmt.Errorf("%w: authoritative global-effect account snapshot is stale", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.02ae086a", "intent plan became stale before dispatch: authoritative global-effect account snapshot is stale", nil))
	}
	effect, found := inventory.GlobalEffects[request.GlobalEffectID]
	if !found || !effect.ActiveAt(now) || effect.EndsAt.Sub(now) <= autoBoosterPurchaseWindowGuard || effect.EndsAt.Unix() != request.ExpectedEndsAtUnix {
		return request, Localization.WithError(fmt.Errorf("%w: daily fortress-speed effect window changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.b6404132", "intent plan became stale before dispatch: daily fortress-speed effect window changed", nil))
	}
	if automation, found := input.State.Automations["autoBooster"]; found {
		if cursor, exists := automation.OperationalCursors[autoBoosterPurchaseCursorKey]; exists && State.SameEventOccurrence(time.Unix(int64(cursor), 0).UTC(), effect.EndsAt) {
			return request, Localization.WithError(fmt.Errorf("%w: daily fortress-speed boost was already accepted for this window", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a8c52fec", "intent plan became stale before dispatch: daily fortress-speed boost was already accepted for this window", nil))
		}
	}
	if record, found := inventory.GlobalEffectPurchases[request.GlobalEffectID]; found && State.SameEventOccurrence(record.OccurrenceEndsAt, effect.EndsAt) {
		ownedPending := record.Outcome == State.GlobalEffectPurchaseUnresolved && pendingOperationID != "" && record.OperationID == pendingOperationID
		if !ownedPending && (record.Outcome == State.GlobalEffectPurchaseUnresolved || record.Outcome == State.GlobalEffectPurchaseAccepted || record.Outcome == State.GlobalEffectPurchaseConfirmed) {
			return request, Localization.WithError(fmt.Errorf("%w: a purchase is already unresolved, accepted, or active for this window", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d683f550", "intent plan became stale before dispatch: a purchase is already unresolved, accepted, or active for this window", nil))
		}
	}
	status, found := inventory.GlobalEffectBoosts[request.GlobalEffectID]
	if !found || status.GlobalEffectID != request.GlobalEffectID || !State.SameEventOccurrence(status.OccurrenceEndsAt, effect.EndsAt) || status.ObservedAt.IsZero() || !status.ObservedAt.Equal(baseline) || status.ConnectionGeneration != input.State.Session.ConnectionGeneration {
		return request, Localization.WithError(fmt.Errorf("%w: current boosted-effect status is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a9db7e58", "intent plan became stale before dispatch: current boosted-effect status is unavailable", nil))
	}
	if status.Boosted {
		return request, Localization.WithError(fmt.Errorf("%w: daily fortress-speed boost is already active", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.b3d62742", "intent plan became stale before dispatch: daily fortress-speed boost is already active", nil))
	}
	offer, found := inventory.GlobalEffectBoosterOffers[request.GlobalEffectID]
	if !found || offer.GlobalEffectID != request.GlobalEffectID || offer.RubyCost != request.ExpectedRubyCost || offer.RubyCost != GameData.FortressDailyBoosterRubyCost || offer.BonusValue != request.ExpectedBonusValue || offer.BonusValue <= 0 {
		return request, Localization.WithError(fmt.Errorf("%w: live fortress-speed offer or price changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.f728d43c", "intent plan became stale before dispatch: live fortress-speed offer or price changed", nil))
	}
	resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
	observation := input.State.Player.ResourceObservations[State.ResourceID(resourceID)]
	if !found || resourceID <= 0 || request.ExpectedRubyResourceID > 0 && request.ExpectedRubyResourceID != resourceID || observation.ObservedAt.IsZero() || !observation.ObservedAt.Equal(baseline) || observation.ConnectionGeneration != input.State.Session.ConnectionGeneration {
		return request, Localization.WithError(fmt.Errorf("%w: a fresh current-session ruby balance is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.8eb2bf6c", "intent plan became stale before dispatch: a fresh current-session ruby balance is unavailable", nil))
	}
	rubyValue, found := input.State.Player.Resources[State.ResourceID(resourceID)]
	if !found {
		return request, Localization.WithError(fmt.Errorf("ruby balance is unavailable"), Localization.New("server.app.ruby_balance_is_unavailable.f542080d", "ruby balance is unavailable", nil))
	}
	rubies := int64(math.Floor(rubyValue))
	if rubies != request.ExpectedRubyBalance {
		return request, Localization.WithError(fmt.Errorf("%w: ruby balance changed before purchase", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.f20476a7", "intent plan became stale before dispatch: ruby balance changed before purchase", nil))
	}
	if rubies-request.MinimumRubyReserve < offer.RubyCost {
		return request, Localization.WithError(fmt.Errorf("%w: purchase would cross the configured ruby reserve", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.bd9d8cbe", "intent plan became stale before dispatch: purchase would cross the configured ruby reserve", nil))
	}
	return request, nil
}

type autoBoosterRuntimeSettings struct {
	Version            int   `json:"version"`
	CheckIntervalSec   int   `json:"checkIntervalSec"`
	RubyCostCeiling    int64 `json:"rubyCostCeiling"`
	MinimumRubyReserve int64 `json:"minimumRubyReserve"`
}

func (application *Application) validateAutoBoosterControls(now time.Time, request autoBoosterPurchaseRequest) error {
	if application == nil || application.Configuration == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("Auto Booster runtime controls are unavailable"), Localization.New("server.app.auto_booster_runtime_controls.6c5a358a", "Auto Booster runtime controls are unavailable", nil))
	}
	if application.automationLocked() {
		return Localization.WithError(fmt.Errorf("%w: Bot Lock is enabled", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.643fc5fd", "intent plan became stale before dispatch: Bot Lock is enabled", nil))
	}
	var enabled map[string]json.RawMessage
	raw, found := application.Configuration.Section("automation.enabled")
	if !found || json.Unmarshal(raw, &enabled) != nil || !autoBoosterControlEnabled(enabled["auto_booster"], now) {
		return Localization.WithError(fmt.Errorf("%w: Auto Booster is disabled", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.9d9fc745", "intent plan became stale before dispatch: Auto Booster is disabled", nil))
	}
	var settings autoBoosterRuntimeSettings
	raw, found = application.Configuration.Section("automation.autoBooster")
	if !found || json.Unmarshal(raw, &settings) != nil || settings.Version != 1 || settings.CheckIntervalSec < 30 || settings.CheckIntervalSec > 3600 ||
		settings.CheckIntervalSec != request.ExpectedCheckIntervalSec || settings.RubyCostCeiling != GameData.FortressDailyBoosterRubyCost ||
		settings.MinimumRubyReserve < 0 || settings.MinimumRubyReserve != request.MinimumRubyReserve {
		return Localization.WithError(fmt.Errorf("%w: Auto Booster settings changed or are invalid", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.0a4459eb", "intent plan became stale before dispatch: Auto Booster settings changed or are invalid", nil))
	}
	session := application.State.Session()
	if !session.LoggedIn || !session.SocketReady {
		return Localization.WithError(fmt.Errorf("%w: game session is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.91f71a6e", "intent plan became stale before dispatch: game session is unavailable", nil))
	}
	return nil
}

func autoBoosterControlEnabled(raw json.RawMessage, now time.Time) bool {
	var enabled bool
	if json.Unmarshal(raw, &enabled) == nil {
		return enabled
	}
	var control struct {
		Enabled   bool       `json:"enabled"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if json.Unmarshal(raw, &control) != nil || !control.Enabled {
		return false
	}
	return control.ExpiresAt == nil || now.Before(control.ExpiresAt.UTC())
}

func (application *Application) armAutoBoosterPurchase(ctx context.Context, arguments json.RawMessage) error {
	return application.mutateAutoBoosterPurchase(ctx, arguments, "arm")
}
func (application *Application) finalizeAutoBoosterDispatch(ctx context.Context, arguments json.RawMessage) error {
	return application.mutateAutoBoosterPurchase(ctx, arguments, "dispatch")
}
func (application *Application) disarmAutoBoosterPurchase(ctx context.Context, arguments json.RawMessage) error {
	return application.mutateAutoBoosterPurchase(ctx, arguments, "disarm")
}
func (application *Application) rejectAutoBoosterPurchase(ctx context.Context, arguments json.RawMessage) error {
	return application.mutateAutoBoosterPurchase(ctx, arguments, "reject")
}

func (application *Application) mutateAutoBoosterPurchase(ctx context.Context, arguments json.RawMessage, action string) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Auto Booster state is unavailable"), Localization.New("server.app.auto_booster_state_is.74c02e9a", "Auto Booster state is unavailable", nil))
	}
	var decoded autoBoosterPurchaseRequest
	if err := decodeIntentArguments(arguments, &decoded); err != nil {
		return err
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	now := time.Now().UTC()
	if action == "arm" || action == "dispatch" {
		if err := application.validateAutoBoosterControls(now, decoded); err != nil {
			return err
		}
	}
	metadata := Outbound.MetadataFromContext(ctx)
	operationID, responseToken := strings.TrimSpace(metadata.OperationID), strings.TrimSpace(metadata.ResponseToken)
	event, err := application.State.ApplyComponents(State.Components(State.ComponentEventScores), func(gameState *State.GameState) ([]string, bool, error) {
		inventory := gameState.EventScores.Inventory
		inventory.GlobalEffectPurchases = cloneAutoBoosterPurchaseRecords(inventory.GlobalEffectPurchases)
		record, found := inventory.GlobalEffectPurchases[decoded.GlobalEffectID]
		matches := found && record.OperationID == operationID && (record.ResponseToken == "" || responseToken == "" || record.ResponseToken == responseToken)
		switch action {
		case "arm":
			if _, err := autoBoosterPurchaseContext(Intent.PlanningContext{State: *gameState, GameData: gameData}, arguments, now, true); err != nil {
				return nil, false, err
			}
			resourceID, _ := gameData.ResourceIDForJSONKey("C2")
			observation := gameState.Player.ResourceObservations[State.ResourceID(resourceID)]
			record = State.GlobalEffectPurchaseRecord{GlobalEffectID: decoded.GlobalEffectID, OccurrenceEndsAt: time.Unix(decoded.ExpectedEndsAtUnix, 0).UTC(), ExpiresAt: time.Unix(decoded.ExpectedEndsAtUnix, 0).UTC(), QuotedRubyCost: decoded.ExpectedRubyCost, QuotedBonusValue: decoded.ExpectedBonusValue, MinimumRubyReserve: decoded.MinimumRubyReserve, RubyBefore: decoded.ExpectedRubyBalance, RubyBeforeObservedAt: observation.ObservedAt, RequestedAt: now, RequestOpcode: "agb", OperationID: operationID, ResponseToken: responseToken, ConnectionGeneration: gameState.Session.ConnectionGeneration, DebitUnverified: true, Outcome: State.GlobalEffectPurchaseUnresolved, Detail: "Boost purchase is being dispatched; its outcome is unresolved until the game replies"}
		case "dispatch":
			if !matches || record.Outcome != State.GlobalEffectPurchaseUnresolved {
				return nil, false, Localization.WithError(fmt.Errorf("%w: the boost purchase marker changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c5be131c", "intent plan became stale before dispatch: the boost purchase marker changed before dispatch", nil))
			}
			if _, err := autoBoosterPurchaseContextForOperation(Intent.PlanningContext{State: *gameState, GameData: gameData}, arguments, now, true, operationID); err != nil {
				return nil, false, err
			}
			if !record.DispatchedAt.IsZero() {
				return nil, false, nil
			}
			record.DispatchedAt = now
			record.Detail = "Boost purchase was dispatched; awaiting the game result"
		case "disarm":
			if !matches || record.Outcome != State.GlobalEffectPurchaseUnresolved {
				return nil, false, nil
			}
			delete(inventory.GlobalEffectPurchases, decoded.GlobalEffectID)
			if gameState.ReplaceEventInventory(inventory) {
				return []string{"events", "event-scores", "global-effects"}, true, nil
			}
			return nil, false, nil
		case "reject":
			if !matches || record.Outcome == State.GlobalEffectPurchaseAccepted || record.Outcome == State.GlobalEffectPurchaseConfirmed {
				return nil, false, nil
			}
			record.Outcome, record.ResultObservedAt, record.Detail = State.GlobalEffectPurchaseRejected, now, "The game explicitly rejected the boost purchase"
		default:
			return nil, false, Localization.WithError(fmt.Errorf("unknown Auto Booster purchase mutation %q", action), Localization.New("server.app.unknown_auto_booster_purchase.0a232e23", "unknown Auto Booster purchase mutation {p0}", Localization.Params{"p0": fmt.Sprintf("%q", action)}))
		}
		if inventory.GlobalEffectPurchases == nil {
			inventory.GlobalEffectPurchases = map[int64]State.GlobalEffectPurchaseRecord{}
		}
		inventory.GlobalEffectPurchases[decoded.GlobalEffectID] = record
		if !gameState.ReplaceEventInventory(inventory) {
			return nil, false, nil
		}
		return []string{"events", "event-scores", "global-effects"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) reconcileAutoBoosterPurchase(ctx context.Context, _ json.RawMessage) error {
	if application == nil || application.State == nil || application.Intents == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Auto Booster reconciliation is unavailable"), Localization.New("server.app.auto_booster_reconciliation_is.e97b7aa6", "Auto Booster reconciliation is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentEventScores), func(gameState *State.GameState) ([]string, bool, error) {
		inventory := gameState.EventScores.Inventory
		inventory.GlobalEffectPurchases = cloneAutoBoosterPurchaseRecords(inventory.GlobalEffectPurchases)
		record, found := inventory.GlobalEffectPurchases[GameData.FortressDailyGlobalEffectID]
		if !found || record.Outcome != State.GlobalEffectPurchaseUnresolved || record.OperationID == "" || record.DispatchedAt.IsZero() {
			return nil, false, nil
		}
		receipt, found := application.Intents.Operation(record.OperationID)
		if !found || !receipt.Terminal() {
			return nil, false, nil
		}
		if acceptedAt, accepted := autoBoosterAcceptedExchange(receipt); accepted {
			code := 0
			record.ResultCode = &code
			record.ResultObservedAt = acceptedAt
			record.Outcome = State.GlobalEffectPurchaseAccepted
			record.DebitUnverified = true
			record.Detail = "The durable operation receipt confirms that the game accepted the boost purchase; awaiting active-state confirmation"
			updateGlobalEffectPurchaseRubyEvidenceForApp(&record, gameState, gameData)
			inventory.GlobalEffectPurchases[record.GlobalEffectID] = record
			if !gameState.ReplaceEventInventory(inventory) {
				return nil, false, nil
			}
			return []string{"events", "event-scores", "global-effects"}, true, nil
		}
		if receipt.Status != Intent.StatusIndeterminate && receipt.Status != Intent.StatusPartiallySucceeded && receipt.Status != Intent.StatusFailed {
			return nil, false, nil
		}
		baseline := inventory.GlobalEffectBaselineObservedAt
		status, statusFound := inventory.GlobalEffectBoosts[record.GlobalEffectID]
		if baseline.IsZero() || !baseline.After(record.DispatchedAt) || inventory.GlobalEffectBaselineGeneration != gameState.Session.ConnectionGeneration ||
			!statusFound || !status.ObservedAt.Equal(baseline) || !State.SameEventOccurrence(status.OccurrenceEndsAt, record.OccurrenceEndsAt) || status.Boosted {
			return nil, false, nil
		}
		updateGlobalEffectPurchaseRubyEvidenceForApp(&record, gameState, gameData)
		record.Outcome = State.GlobalEffectPurchaseRejected
		record.ResultObservedAt = baseline
		record.DebitUnverified = true
		record.Detail = "A later complete account snapshot reports the boost inactive after the unresolved operation ended"
		inventory.GlobalEffectPurchases[record.GlobalEffectID] = record
		if !gameState.ReplaceEventInventory(inventory) {
			return nil, false, nil
		}
		return []string{"events", "event-scores", "global-effects"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func autoBoosterAcceptedExchange(receipt Intent.Receipt) (time.Time, bool) {
	for _, exchange := range receipt.Exchanges {
		if !strings.EqualFold(exchange.Command.Opcode, "agb") || exchange.Response == nil ||
			exchange.Response.ResponseCode == nil || *exchange.Response.ResponseCode != 0 {
			continue
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(exchange.Command.Payload, &payload) != nil || len(payload) != 1 {
			continue
		}
		globalEffectID, valid := strictJSONInt64(payload["GEID"])
		if !valid || globalEffectID != GameData.FortressDailyGlobalEffectID {
			continue
		}
		return exchange.Response.ReceivedAt.UTC(), true
	}
	return time.Time{}, false
}

func strictJSONInt64(raw json.RawMessage) (int64, bool) {
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&number) != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	return value, err == nil
}

func cloneAutoBoosterPurchaseRecords(input map[int64]State.GlobalEffectPurchaseRecord) map[int64]State.GlobalEffectPurchaseRecord {
	result := make(map[int64]State.GlobalEffectPurchaseRecord, len(input))
	for id, record := range input {
		if record.ResultCode != nil {
			code := *record.ResultCode
			record.ResultCode = &code
		}
		result[id] = record
	}
	return result
}

func updateGlobalEffectPurchaseRubyEvidenceForApp(record *State.GlobalEffectPurchaseRecord, gameState *State.GameState, gameData *GameData.Store) {
	if record == nil || gameState == nil || gameData == nil {
		return
	}
	resourceID, found := gameData.ResourceIDForJSONKey("C2")
	observation := gameState.Player.ResourceObservations[State.ResourceID(resourceID)]
	value, valueFound := gameState.Player.Resources[State.ResourceID(resourceID)]
	minimumObservedAt := record.RubyBeforeObservedAt
	if record.DispatchedAt.After(minimumObservedAt) {
		minimumObservedAt = record.DispatchedAt
	}
	if !found || resourceID <= 0 || !valueFound || observation.ObservedAt.IsZero() || !observation.ObservedAt.After(minimumObservedAt) || observation.ConnectionGeneration != record.ConnectionGeneration {
		return
	}
	record.RubyAfter = int64(math.Floor(value))
	record.RubyAfterKnown = true
	record.RubyAfterObservedAt = observation.ObservedAt
	record.ObservedRubyChange = record.RubyBefore - record.RubyAfter
}
