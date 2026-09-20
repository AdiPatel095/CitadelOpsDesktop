package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type autoBuyerHistoryRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
}

type autoBuyerBoostersRefreshRequest struct {
	FeastContext      bool      `json:"feastContext"`
	FeastRefreshAfter time.Time `json:"feastRefreshAfter,omitempty"`
}

type autoBuyerPackagePurchaseRequest struct {
	SourceCastleID           State.CastleID  `json:"sourceCastleId"`
	ShopID                   string          `json:"shopId"`
	PackageID                State.PackageID `json:"packageId"`
	Amount                   int64           `json:"amount"`
	TargetPurchasesPerReset  int64           `json:"targetPurchasesPerReset"`
	MinimumBalanceReserve    int64           `json:"minimumBalanceReserve"`
	AllowRubyPackages        bool            `json:"allowRubyPackages"`
	MaximumRubySpendPerReset int64           `json:"maximumRubySpendPerReset"`
	MinimumRubyReserve       int64           `json:"minimumRubyReserve"`
	ExpectedPurchasedBefore  int64           `json:"expectedPurchasedBefore"`
	ExpectedBalanceBefore    int64           `json:"expectedBalanceBefore"`
}

type autoBuyerSpecialistPurchaseRequest struct {
	SpecialistID               int       `json:"specialistId"`
	MinimumDays                int       `json:"minimumDays"`
	MaximumRubyCostPerPurchase int64     `json:"maximumRubyCostPerPurchase"`
	MinimumRubyReserve         int64     `json:"minimumRubyReserve"`
	ExpectedExpiresAtUnix      int64     `json:"expectedExpiresAtUnix"`
	ExpectedRubyBalance        int64     `json:"expectedRubyBalance"`
	ExpectedRubyObservedAt     time.Time `json:"expectedRubyObservedAt"`
	ExpectedSessionGeneration  uint64    `json:"expectedSessionGeneration"`
	ExpectedRubyResourceID     int64     `json:"expectedRubyResourceId"`
	AttemptAfter               time.Time `json:"attemptAfter"`
	HistoryRefreshSec          int       `json:"historyRefreshSec"`
}

type autoBuyerFeastPurchaseRequest struct {
	FeastID                    int64           `json:"feastId"`
	MinimumRemainingHours      int             `json:"minimumRemainingHours"`
	SourceCastleID             State.CastleID  `json:"sourceCastleId"`
	ExpectedSourceKingdomID    State.KingdomID `json:"expectedSourceKingdomId"`
	MinimumFoodReserve         int64           `json:"minimumFoodReserve"`
	AllowRubies                bool            `json:"allowRubies"`
	MaximumRubyCostPerPurchase int64           `json:"maximumRubyCostPerPurchase"`
	MinimumRubyReserve         int64           `json:"minimumRubyReserve"`
	ExpectedActiveFeastID      int64           `json:"expectedActiveFeastId"`
	ExpectedExpiresAtUnix      int64           `json:"expectedExpiresAtUnix"`
	ExpectedExpiresAt          time.Time       `json:"expectedExpiresAt,omitempty"`
	ExpectedBalanceBefore      *int64          `json:"expectedBalanceBefore,omitempty"`
	ExpectedEffectiveCost      *int64          `json:"expectedEffectiveCost,omitempty"`
	AttemptAfter               time.Time       `json:"attemptAfter,omitempty"`
	FeastRefreshAfter          time.Time       `json:"feastRefreshAfter,omitempty"`
	HistoryRefreshSec          int             `json:"historyRefreshSec"`
}

type autoBuyerFeastRoute struct {
	FeastID   int64           `json:"T"`
	CastleID  State.CastleID  `json:"CID"`
	KingdomID State.KingdomID `json:"KID"`
	Preflight bool            `json:"_citadelFeastPreflight,omitempty"`
}

func (application *Application) registerAutoBuyerIntents() error {
	if err := application.Intents.RegisterCommandDependencies("bfs", application.resolveAutoBuyerFeastCommandDependencies); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("auto_buyer.feast.purchase.build", resolveAutoBuyerFeastPurchaseStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("auto_buyer.specialist.purchase.build", resolveAutoBuyerSpecialistPurchaseStep); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "autoBuyer.package.history", Description: "Refresh server purchase counters used to detect shop stock resets",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{"sourceCastleId":123}`), Planner: planAutoBuyerPackageHistory,
		},
		{
			Name: "autoBuyer.boosters.refresh", Description: "Refresh specialist and feast timers before evaluating renewal floors",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{}`), Planner: planAutoBuyerBoostersRefresh,
		},
		{
			Name: "autoBuyer.package.purchase", Description: "Purchase a bounded official package after a fresh stock, event, price, and reserve guard",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"sourceCastleId":123,"shopId":"master-blacksmith","packageId":456,"amount":1,"targetPurchasesPerReset":1}`), Planner: planAutoBuyerPackagePurchase,
		},
		{
			Name: "autoBuyer.specialist.purchase", Description: "Renew one supported specialist by one official seven-day period within a ruby ceiling",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"specialistId":0,"minimumDays":14,"maximumRubyCostPerPurchase":625}`), Planner: planAutoBuyerSpecialistPurchase,
		},
		{
			Name: "autoBuyer.specialist.reconcile", Description: "Refresh authoritative specialist state after an unresolved purchase without spending",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{"specialistId":0}`), Planner: planAutoBuyerSpecialistReconcile,
		},
		{
			Name: "autoBuyer.feast.purchase", Description: "Start or extend one official feast within configured food or ruby reserves",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"feastId":0,"minimumRemainingHours":12,"sourceCastleId":123}`), Planner: planAutoBuyerFeastPurchase,
		},
		{
			Name: "autoBuyer.feast.reconcile", Description: "Refresh feast state after an incomplete or uncertain purchase",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{"feastId":0,"sourceCastleId":123}`), Planner: planAutoBuyerFeastReconcile,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	actions := map[string]Intent.Action{
		"auto_buyer.package.guard":          application.guardAutoBuyerPackagePurchase,
		"auto_buyer.package.verify":         application.verifyAutoBuyerPackagePurchase,
		"auto_buyer.specialist.guard":       application.guardAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.verify":      application.verifyAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.arm":         application.armAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.disarm":      application.disarmAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.reject":      application.rejectAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.dispatch":    application.guardAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.reconcile":   application.reconcileAutoBuyerSpecialistPurchase,
		"auto_buyer.feast.purchase.arm":     application.armAutoBuyerFeastPurchase,
		"auto_buyer.feast.purchase.disarm":  application.disarmAutoBuyerFeastPurchase,
		"auto_buyer.feast.reconcile.mark":   application.markAutoBuyerFeastReconciliation,
		"auto_buyer.feast.reconcile.verify": application.verifyAutoBuyerFeastReconciliation,
		"auto_buyer.feast.refresh.verify":   application.verifyAutoBuyerFeastRefresh,
		"auto_buyer.feast.verify":           application.verifyAutoBuyerFeastPurchase,
	}
	for name, action := range actions {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	return nil
}

func planAutoBuyerPackageHistory(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request autoBuyerHistoryRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, err := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(map[string]any{"CID": source.ID, "KID": source.KingdomID})
	step := shopCommandStep("Refresh Auto Buyer package counters", "gbc", payload, 0)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	steps := castleContextSteps(input, source)
	steps = append(steps, step)
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:purchase-history", "castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Refresh Auto Buyer stock counters from %s", castleLabel(source)), Steps: steps,
	}, nil
}

func planAutoBuyerBoostersRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request autoBuyerBoostersRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	claims := []string{"shop", "market:boosters"}
	steps := []Intent.Step{}
	if request.FeastContext {
		request.FeastRefreshAfter = time.Now().UTC()
		costReduction := commandStep("Refresh feast cost reduction", "fce", json.RawMessage(`{}`), "fce")
		costReduction.ResponseBarrier = Intent.ResponseBarrierCommitted
		castleDetails := commandStep("Refresh feast castle resources", "dcl", json.RawMessage(`{"CD":1}`), "dcl")
		castleDetails.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, Intent.RebuildOnResume(costReduction), Intent.RebuildOnResume(castleDetails))
		claims = append(claims, "castle-directory", "account-resources")
	}
	boosters := shopCommandStep("Refresh specialist and feast timers", "boi", json.RawMessage(`{}`), 0)
	boosters.ResponseBarrier = Intent.ResponseBarrierCommitted
	steps = append(steps, Intent.RebuildOnResume(boosters))
	summary := "Refresh Auto Buyer specialist timers"
	if request.FeastContext {
		summary = "Refresh Auto Buyer feast cost, castle resources, and timers"
		resolved, _ := json.Marshal(request)
		steps = append(steps, Intent.RebuildOnResume(Intent.Step{
			Name: "Verify Auto Buyer feast refresh", Action: "auto_buyer.feast.refresh.verify", ActionArguments: resolved,
		}))
	}
	return Intent.Plan{
		Claims: claims, Summary: summary, Steps: steps,
	}, nil
}

func planAutoBuyerPackagePurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, product, err := autoBuyerPackagePurchaseContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	historyPayload, _ := json.Marshal(map[string]any{"CID": source.ID, "KID": source.KingdomID})
	historyBefore := shopCommandStep("Refresh package counter before purchase", "gbc", historyPayload, 0)
	historyBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	historyAfter := shopCommandStep("Verify package counter after purchase", "gbc", historyPayload, 0)
	historyAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	purchaseCastleID := int64(-1)
	purchaseKingdomID := State.KingdomID(0)
	if product.Price.Scope == GameData.AutoBuyerPriceCastleResource {
		purchaseCastleID = int64(source.ID)
		purchaseKingdomID = source.KingdomID
	}
	payload, _ := json.Marshal(struct {
		ProductID State.PackageID `json:"PID"`
		BuildType int64           `json:"BT"`
		TableID   int64           `json:"TID"`
		Amount    int64           `json:"AMT"`
		KingdomID State.KingdomID `json:"KID"`
		CastleID  int64           `json:"AID"`
		Premium   int64           `json:"PC2"`
		BuyAll    int64           `json:"BA"`
		Power     int64           `json:"PWR"`
		Position  int64           `json:"_PO"`
	}{State.PackageID(product.PackageID), 0, product.TableID, request.Amount, purchaseKingdomID, purchaseCastleID, -1, 0, 0, -1})
	steps := castleContextSteps(input, source)
	steps = append(steps, historyBefore)
	if product.Price.Scope == GameData.AutoBuyerPriceCastleResource {
		resourcePayload, _ := json.Marshal(map[string]any{"AID": source.ID, "KID": source.KingdomID})
		resourceRefresh := commandStep("Refresh package source resources before purchase", "grc", resourcePayload, "grc")
		resourceRefresh.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, resourceRefresh)
	}
	purchase := shopCommandStep("Purchase "+product.Name, "sbp", payload, 0)
	purchase.FinalDispatchAction = "auto_buyer.package.guard"
	purchase.FinalDispatchArguments = append(json.RawMessage(nil), resolved...)
	steps = append(steps,
		Intent.RebuildOnResume(Intent.Step{Name: "Recheck Auto Buyer package purchase", Action: "auto_buyer.package.guard", ActionArguments: resolved}),
		purchase, historyAfter,
		Intent.RebuildOnResume(Intent.Step{Name: "Verify Auto Buyer package purchase", Action: "auto_buyer.package.verify", ActionArguments: resolved}),
	)
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:table:" + strconv.FormatInt(product.TableID, 10), "shop:purchase-history", "account-resources",
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Buy %d x %s for %d %s", request.Amount, product.Name, request.Amount*product.Price.Amount, product.Price.Name),
		Steps:   steps,
	}, nil
}

func planAutoBuyerSpecialistPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	now := time.Now().UTC()
	request, specialist, err := autoBuyerSpecialistPurchaseContext(input, arguments, now)
	if err != nil {
		return Intent.Plan{}, err
	}
	request.AttemptAfter = now
	resolved, _ := json.Marshal(request)
	refreshBefore := shopCommandStep("Refresh specialist timer before renewal", "boi", json.RawMessage(`{}`), 0)
	refreshBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	refreshAfter := shopCommandStep("Refresh specialist timer after renewal", "boi", json.RawMessage(`{}`), 0)
	refreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "market:boosters", "account-resources", "specialist:" + strconv.Itoa(request.SpecialistID)},
		Summary: fmt.Sprintf("Renew %s by 7 days within a %d-ruby ceiling", specialist.Name, request.MaximumRubyCostPerPurchase),
		Steps: []Intent.Step{
			refreshBefore,
			{Name: "Renew " + specialist.Name, Resolver: "auto_buyer.specialist.purchase.build", ResolverArguments: resolved,
				AwaitOpcode: specialist.Opcode, TimeoutMillis: 10_000, SuccessCodes: []int{0}, CaptureResponse: true,
				ResponseBarrier: Intent.ResponseBarrierCommitted, ResponseProjectionFailureIndeterminate: true},
			refreshAfter,
			Intent.RebuildOnResume(Intent.Step{Name: "Verify specialist renewal", Action: "auto_buyer.specialist.verify", ActionArguments: resolved}),
		},
	}, nil
}

func resolveAutoBuyerSpecialistPurchaseStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	now := time.Now().UTC()
	request, specialist, err := autoBuyerSpecialistPurchaseContext(input, arguments, now)
	if err != nil {
		return Intent.Step{}, err
	}
	resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
	if !found || resourceID <= 0 {
		return Intent.Step{}, fmt.Errorf("official ruby resource is unavailable")
	}
	request.ExpectedExpiresAtUnix = autoBuyerIntentUnix(input.State.Market.Boosters[request.SpecialistID].ExpiresAt)
	request.ExpectedRubyBalance = int64(math.Floor(input.State.Player.Resources[State.ResourceID(resourceID)]))
	request.ExpectedRubyObservedAt = input.State.Player.ResourceObservations[State.ResourceID(resourceID)].ObservedAt
	request.ExpectedSessionGeneration = input.State.Session.ConnectionGeneration
	request.ExpectedRubyResourceID = resourceID
	request.AttemptAfter = now
	resolved, _ := json.Marshal(request)
	payload := json.RawMessage(`{"PO":-1}`)
	if specialist.Opcode == "ovs" {
		payload, _ = json.Marshal(map[string]any{"T": specialist.ResourceType, "PO": -1})
	}
	step := shopCommandStep("Renew "+specialist.Name, specialist.Opcode, payload, 0)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	step.CaptureResponse = true
	step.PreDispatchAction = "auto_buyer.specialist.arm"
	step.PreDispatchArguments = resolved
	step.FinalDispatchAction = "auto_buyer.specialist.dispatch"
	step.FinalDispatchArguments = resolved
	step.DefinitiveSendFailureAction = "auto_buyer.specialist.disarm"
	step.DefinitiveSendFailureArguments = resolved
	step.DefinitiveResponseFailureAction = "auto_buyer.specialist.reject"
	step.DefinitiveResponseFailureArguments = resolved
	step.ResponseProjectionFailureIndeterminate = true
	return step, nil
}

func planAutoBuyerSpecialistReconcile(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SpecialistID int `json:"specialistId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil || request.SpecialistID < 0 {
		return Intent.Plan{}, fmt.Errorf("specialist reconciliation request is invalid")
	}
	refresh := shopCommandStep("Refresh unresolved specialist timer", "boi", json.RawMessage(`{}`), 0)
	refresh.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{Claims: []string{"shop", "market:boosters", "account-resources"}, Summary: "Reconcile unresolved specialist purchase", Steps: []Intent.Step{
		Intent.RebuildOnResume(refresh), Intent.RebuildOnResume(Intent.Step{Name: "Reconcile specialist purchase", Action: "auto_buyer.specialist.reconcile", ActionArguments: arguments}),
	}}, nil
}

func planAutoBuyerFeastPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	plannedAt := time.Now().UTC()
	request, source, feast, err := autoBuyerFeastPurchaseContext(input, arguments, plannedAt, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.AttemptAfter.IsZero() {
		request.AttemptAfter = plannedAt
	}
	effectiveCost, err := autoBuyerIntentFeastCost(input.State, feast, request.HistoryRefreshSec, plannedAt)
	if err != nil {
		return Intent.Plan{}, err
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, feast.Price)
	if !available {
		return Intent.Plan{}, fmt.Errorf("%s balance is unavailable", feast.Price.Name)
	}
	request.ExpectedBalanceBefore = &balance
	request.ExpectedEffectiveCost = &effectiveCost
	request.ExpectedExpiresAt = input.State.Market.Feast.ExpiresAt
	request.FeastRefreshAfter = plannedAt
	resolved, _ := json.Marshal(request)
	routePayload, _ := json.Marshal(autoBuyerFeastRoute{
		FeastID: feast.ID, CastleID: source.ID, KingdomID: source.KingdomID, Preflight: true,
	})
	boosterRefreshAfter := shopCommandStep("Refresh feast timer after purchase", "boi", json.RawMessage(`{}`), 0)
	boosterRefreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	castleRefreshAfter := commandStep("Refresh feast castle resources after purchase", "dcl", json.RawMessage(`{"CD":1}`), "dcl")
	castleRefreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	purchase := Intent.Step{
		Name:     "Refresh feast context and start or extend " + feast.Name,
		Resolver: "auto_buyer.feast.purchase.build", ResolverArguments: resolved,
		AwaitOpcode: "bfs", TimeoutMillis: 10_000, SuccessCodes: []int{0}, CaptureResponse: true,
		ResponseBarrier:                        Intent.ResponseBarrierCommitted,
		ResponseProjectionFailureIndeterminate: true,
		CommandDependencies:                    &Intent.CommandDependencyRequest{Opcode: "bfs", Payload: routePayload},
	}
	return Intent.Plan{
		Claims: []string{
			"shop", "market:boosters", "castle-focus", "castle-directory", "account-resources", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Start or extend %s for %d %s", feast.Name, effectiveCost, feast.Price.Name),
		Steps: []Intent.Step{
			purchase,
			Intent.RebuildOnResume(boosterRefreshAfter), Intent.RebuildOnResume(castleRefreshAfter),
			Intent.RebuildOnResume(Intent.Step{Name: "Verify feast purchase", Action: "auto_buyer.feast.verify", ActionArguments: resolved}),
		},
	}, nil
}

func planAutoBuyerFeastReconcile(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.FeastID < 0 || request.SourceCastleID <= 0 || request.AttemptAfter.IsZero() {
		return Intent.Plan{}, fmt.Errorf("feast reconciliation boundary is invalid")
	}
	boosterRefresh := shopCommandStep("Reconcile feast timer after incomplete purchase", "boi", json.RawMessage(`{}`), 0)
	boosterRefresh.ResponseBarrier = Intent.ResponseBarrierCommitted
	castleRefresh := commandStep("Reconcile feast castle resources after incomplete purchase", "dcl", json.RawMessage(`{"CD":1}`), "dcl")
	castleRefresh.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "market:boosters", "castle-directory", "account-resources", "castle:" + strconv.FormatInt(int64(request.SourceCastleID), 10)},
		Summary: "Reconcile an incomplete feast purchase before another attempt",
		Steps: []Intent.Step{
			Intent.RebuildOnResume(Intent.Step{
				Name: "Preserve unresolved feast purchase", Action: "auto_buyer.feast.reconcile.mark", ActionArguments: arguments,
			}),
			Intent.RebuildOnResume(boosterRefresh), Intent.RebuildOnResume(castleRefresh),
			Intent.RebuildOnResume(Intent.Step{
				Name: "Verify feast purchase reconciliation", Action: "auto_buyer.feast.reconcile.verify", ActionArguments: arguments,
			}),
		},
	}, nil
}

func (application *Application) resolveAutoBuyerFeastCommandDependencies(
	_ context.Context,
	input Intent.PlanningContext,
	step Intent.Step,
) (Intent.CommandDependencyPlan, error) {
	payload := step.Command.Payload
	if len(payload) == 0 {
		payload = step.Payload
	}
	var route autoBuyerFeastRoute
	if err := json.Unmarshal(payload, &route); err != nil {
		return Intent.CommandDependencyPlan{}, fmt.Errorf("decode feast purchase route: %w", err)
	}
	if route.FeastID < 0 || route.CastleID <= 0 {
		return Intent.CommandDependencyPlan{}, fmt.Errorf("feast purchase route is invalid")
	}
	key := fmt.Sprintf("%d:%d:%d", route.KingdomID, route.CastleID, route.FeastID)
	if !route.Preflight {
		return Intent.CommandDependencyPlan{Key: key}, nil
	}
	costReduction := commandStep("Refresh feast cost reduction before purchase", "fce", json.RawMessage(`{}`), "fce")
	costReduction.ResponseBarrier = Intent.ResponseBarrierCommitted
	feastTimer := shopCommandStep("Refresh feast timer before purchase", "boi", json.RawMessage(`{}`), 0)
	feastTimer.ResponseBarrier = Intent.ResponseBarrierCommitted
	castleDetails := commandStep("Refresh feast castle resources immediately before purchase", "dcl", json.RawMessage(`{"CD":1}`), "dcl")
	castleDetails.ResponseBarrier = Intent.ResponseBarrierCommitted
	source, found := input.State.Castles[route.CastleID]
	if !found || source.ID != route.CastleID || source.KingdomID != route.KingdomID ||
		!GameData.AutoBuyerFeastCastleUsable(input.State, route.CastleID, source) {
		return Intent.CommandDependencyPlan{}, fmt.Errorf("%w: automatic feast source %d is no longer usable", Intent.ErrPlanStale, route.CastleID)
	}
	contextSteps := castleContextSteps(input, source)
	if len(contextSteps) == 0 {
		contextSteps = []Intent.Step{castleRefreshStep("Refresh automatic feast source economy", source)}
	}
	return Intent.CommandDependencyPlan{
		Key: key,
		Steps: append(contextSteps,
			Intent.RebuildOnResume(feastTimer), Intent.RebuildOnResume(costReduction),
			Intent.RebuildOnResume(castleDetails)),
	}, nil
}

func resolveAutoBuyerFeastPurchaseStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	_, source, feast, err := autoBuyerFeastPurchaseContext(input, arguments, time.Now().UTC(), true)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"T": feast.ID, "CID": source.ID, "KID": source.KingdomID, "PO": -1, "PWR": 0,
	})
	step := shopCommandStep("Start or extend "+feast.Name, "bfs", payload, 0)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	step.PreDispatchAction = "auto_buyer.feast.purchase.arm"
	step.PreDispatchArguments = append(json.RawMessage(nil), arguments...)
	step.DefinitiveSendFailureAction = "auto_buyer.feast.purchase.disarm"
	step.DefinitiveSendFailureArguments = append(json.RawMessage(nil), arguments...)
	step.ResponseProjectionFailureIndeterminate = true
	return step, nil
}

func autoBuyerPackagePurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requireFresh bool,
) (autoBuyerPackagePurchaseRequest, State.CastleState, GameData.AutoBuyerPackage, error) {
	var request autoBuyerPackagePurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, err
	}
	request.ShopID = strings.TrimSpace(request.ShopID)
	if request.PackageID <= 0 || request.Amount <= 0 || request.TargetPurchasesPerReset <= 0 ||
		request.MinimumBalanceReserve < 0 || request.MaximumRubySpendPerReset < 0 || request.MinimumRubyReserve < 0 ||
		request.ExpectedPurchasedBefore < 0 || request.ExpectedBalanceBefore < 0 {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, fmt.Errorf("Auto Buyer package request contains an invalid product, amount, target, balance, reserve, or ceiling")
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, fmt.Errorf("official game data is unavailable")
	}
	source, err := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, err
	}
	product, found := input.GameData.AutoBuyerPackage(request.ShopID, int64(request.PackageID))
	if !found {
		return request, source, GameData.AutoBuyerPackage{}, fmt.Errorf("package %d is not in the supported %s Auto Buyer catalog", request.PackageID, request.ShopID)
	}
	if request.TargetPurchasesPerReset > product.Stock {
		return request, source, product, fmt.Errorf("package target exceeds official stock %d", product.Stock)
	}
	if !autoBuyerIntentLevelEligible(input.State.Player, product.MinLevel, product.MaxLevel, product.MinLegendLevel, product.MaxLegendLevel) {
		return request, source, product, fmt.Errorf("package %d is not available at the current player level", request.PackageID)
	}
	purchased, routeErr := validateEventBackedSBP(input, source, eventBackedSBPRequest{
		PackageID: request.PackageID, TableID: product.TableID, Amount: request.Amount,
		Stock: product.Stock, MaxBuyPerClick: product.MaxBuyPerClick,
	}, now, requireFresh)
	if routeErr != nil {
		return request, source, product, routeErr
	}
	if requireFresh {
		if purchased != request.ExpectedPurchasedBefore {
			return request, source, product, fmt.Errorf("%w: package %d purchase count changed from %d to %d", Intent.ErrPlanStale, request.PackageID, request.ExpectedPurchasedBefore, purchased)
		}
		remaining := min(request.TargetPurchasesPerReset, product.Stock) - purchased
		remaining = min(remaining, product.Stock-purchased)
		if product.MaxBuyPerClick > 0 {
			remaining = min(remaining, product.MaxBuyPerClick)
		}
		if request.Amount > remaining || remaining <= 0 {
			return request, source, product, fmt.Errorf("%w: package %d now has only %d configured purchases remaining", Intent.ErrPlanStale, request.PackageID, max(int64(0), remaining))
		}
		balance, available := autoBuyerIntentPriceBalance(input.State, source, product.Price)
		if !available {
			return request, source, product, fmt.Errorf("%s balance is unavailable", product.Price.Name)
		}
		reserve := request.MinimumBalanceReserve
		if product.Price.Premium {
			if !request.AllowRubyPackages || request.MaximumRubySpendPerReset <= 0 {
				return request, source, product, fmt.Errorf("ruby package purchases are not explicitly enabled")
			}
			reserve = max(reserve, request.MinimumRubyReserve)
			if purchased > math.MaxInt64/product.Price.Amount || request.Amount > math.MaxInt64/product.Price.Amount {
				return request, source, product, fmt.Errorf("package ruby amount is too large")
			}
			if (purchased+request.Amount)*product.Price.Amount > request.MaximumRubySpendPerReset {
				return request, source, product, fmt.Errorf("package purchase exceeds the configured per-reset ruby ceiling")
			}
		}
		if request.Amount > math.MaxInt64/product.Price.Amount {
			return request, source, product, fmt.Errorf("package purchase amount is too large")
		}
		required := request.Amount * product.Price.Amount
		if balance-reserve < required {
			return request, source, product, fmt.Errorf("%w: package requires %d %s above reserve but only %d is spendable", Intent.ErrPlanStale, required, product.Price.Name, max(int64(0), balance-reserve))
		}
	}
	return request, source, product, nil
}

func autoBuyerSpecialistPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (autoBuyerSpecialistPurchaseRequest, GameData.AutoBuyerSpecialist, error) {
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, GameData.AutoBuyerSpecialist{}, err
	}
	specialist, found := GameData.AutoBuyerSpecialistByID(request.SpecialistID)
	if !found || request.MinimumDays < 14 || request.MinimumDays > 365 || request.MinimumRubyReserve < 0 ||
		specialist.ValidatedMaximumRubyCost <= 0 || request.MaximumRubyCostPerPurchase < specialist.ValidatedMaximumRubyCost || request.HistoryRefreshSec < 60 || request.HistoryRefreshSec > 3600 ||
		request.ExpectedRubyBalance < 0 {
		return request, specialist, fmt.Errorf("specialist renewal settings are invalid or do not cover the safe official ruby maximum")
	}
	if input.GameData == nil {
		return request, specialist, fmt.Errorf("official game data is unavailable")
	}
	if input.State.Market.SpecialistPurchasePending || input.State.Market.BoostersObservedAt.IsZero() ||
		input.State.Market.BoostersObservedGeneration != input.State.Session.ConnectionGeneration ||
		(!input.State.Session.ChangedAt.IsZero() && input.State.Market.BoostersObservedAt.Before(input.State.Session.ChangedAt)) ||
		input.State.Market.BoostersObservedAt.After(now) || now.Sub(input.State.Market.BoostersObservedAt) >= time.Duration(request.HistoryRefreshSec)*time.Second {
		return request, specialist, fmt.Errorf("%w: specialist timers are stale", Intent.ErrPlanStale)
	}
	booster := input.State.Market.Boosters[request.SpecialistID]
	if !autoBuyerIntentExpiryMatches(booster.ExpiresAt, request.ExpectedExpiresAtUnix) {
		return request, specialist, fmt.Errorf("%w: %s timer changed", Intent.ErrPlanStale, specialist.Name)
	}
	if booster.Permanent || booster.ExpiresAt.After(now.Add(time.Duration(request.MinimumDays)*24*time.Hour)) {
		return request, specialist, fmt.Errorf("%w: %s already meets its configured floor", Intent.ErrPlanStale, specialist.Name)
	}
	resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
	observation := input.State.Player.ResourceObservations[State.ResourceID(resourceID)]
	if !found || resourceID <= 0 || request.ExpectedRubyResourceID > 0 && request.ExpectedRubyResourceID != resourceID ||
		observation.ObservedAt.IsZero() || observation.ObservedAt.After(now) ||
		(!input.State.Session.ChangedAt.IsZero() && observation.ObservedAt.Before(input.State.Session.ChangedAt)) ||
		observation.ConnectionGeneration != input.State.Session.ConnectionGeneration || now.Sub(observation.ObservedAt) > 60*time.Second {
		return request, specialist, fmt.Errorf("%w: a fresh current-session ruby balance is unavailable", Intent.ErrPlanStale)
	}
	rubies := int64(math.Floor(input.State.Player.Resources[State.ResourceID(resourceID)]))
	if !request.ExpectedRubyObservedAt.IsZero() && (!observation.ObservedAt.Equal(request.ExpectedRubyObservedAt) || request.ExpectedSessionGeneration != input.State.Session.ConnectionGeneration || rubies != request.ExpectedRubyBalance) {
		return request, specialist, fmt.Errorf("%w: ruby balance changed before dispatch", Intent.ErrPlanStale)
	}
	if rubies-request.MinimumRubyReserve < specialist.ValidatedMaximumRubyCost {
		return request, specialist, fmt.Errorf("%w: %s requires up to %d rubies above reserve", Intent.ErrPlanStale, specialist.Name, specialist.ValidatedMaximumRubyCost)
	}
	return request, specialist, nil
}

func autoBuyerFeastPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requirePreflight bool,
) (autoBuyerFeastPurchaseRequest, State.CastleState, GameData.AutoBuyerFeast, error) {
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, err
	}
	if request.MinimumRemainingHours <= 0 || request.MinimumRemainingHours > 24*30 || request.MinimumFoodReserve < 0 ||
		request.MinimumRubyReserve < 0 || request.MaximumRubyCostPerPurchase < 0 || request.HistoryRefreshSec < 60 ||
		request.HistoryRefreshSec > 3600 || request.ExpectedEffectiveCost != nil && *request.ExpectedEffectiveCost < 0 ||
		request.ExpectedBalanceBefore != nil && *request.ExpectedBalanceBefore < 0 {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("feast floor, balance, reserve, or ruby ceiling is invalid")
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("official game data is unavailable")
	}
	refreshAge := time.Duration(request.HistoryRefreshSec) * time.Second
	selection, sourceStatus, sourceErr := input.GameData.SelectAutoBuyerFeastSource(input.State, now, refreshAge)
	if sourceErr != nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, sourceErr
	}
	if len(sourceStatus.StaleCandidateIDs) > 0 {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("%w: automatic feast source observations are stale", Intent.ErrPlanStale)
	}
	if selection.Castle.ID <= 0 {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("%w: no owned castle has fresh positive net food production", Intent.ErrPlanStale)
	}
	source := selection.Castle
	if source.ID != request.SourceCastleID || source.KingdomID != request.ExpectedSourceKingdomID {
		return request, source, GameData.AutoBuyerFeast{}, fmt.Errorf("%w: automatic feast source changed to castle %d", Intent.ErrPlanStale, source.ID)
	}
	feast, found := input.GameData.AutoBuyerFeast(request.FeastID)
	if !found {
		return request, source, GameData.AutoBuyerFeast{}, fmt.Errorf("feast %d is not in the supported official catalog", request.FeastID)
	}
	if !feast.AutomaticPurchase.Supported {
		return request, source, feast, fmt.Errorf("%s", feast.AutomaticPurchase.Reason)
	}
	if input.State.Market.FeastPurchasePending {
		return request, source, feast, fmt.Errorf("%w: a previous feast purchase is still awaiting reconciliation", Intent.ErrPlanStale)
	}
	if !autoBuyerIntentLevelEligible(input.State.Player, feast.MinLevel, feast.MaxLevel, 0, 0) {
		return request, source, feast, fmt.Errorf("%s is not available at the current player level", feast.Name)
	}
	if !input.State.Market.Feast.FreshAt(now, input.State.Session.ChangedAt, refreshAge) {
		return request, source, feast, fmt.Errorf("%w: feast timer is stale", Intent.ErrPlanStale)
	}
	current := input.State.Market.Feast
	if requirePreflight {
		if request.FeastRefreshAfter.IsZero() || current.ObservedAt.Before(request.FeastRefreshAfter) {
			return request, source, feast, fmt.Errorf("%w: feast timer was not refreshed immediately before purchase", Intent.ErrPlanStale)
		}
		if !feast.Price.Premium && input.State.Market.FeastCostReductionObservedAt.Before(request.FeastRefreshAfter) {
			return request, source, feast, fmt.Errorf("%w: feast cost was not refreshed immediately before purchase", Intent.ErrPlanStale)
		}
		if source.FoodBalanceObservedAt.Before(request.FeastRefreshAfter) ||
			!autoBuyerIntentObservationFresh(source.FoodBalanceObservedAt, now, input.State.Session.ChangedAt, refreshAge) {
			return request, source, feast, fmt.Errorf("%w: feast food balance was not refreshed immediately before purchase", Intent.ErrPlanStale)
		}
		if source.FoodEconomyObservedAt.Before(request.FeastRefreshAfter) ||
			!autoBuyerIntentObservationFresh(source.FoodEconomyObservedAt, now, input.State.Session.ChangedAt, refreshAge) ||
			source.ContextSnapshotObservedAt.Before(request.FeastRefreshAfter) ||
			!autoBuyerIntentObservationFresh(source.ContextSnapshotObservedAt, now, input.State.Session.ChangedAt, refreshAge) {
			return request, source, feast, fmt.Errorf("%w: feast source economy was not refreshed immediately before purchase", Intent.ErrPlanStale)
		}
	}
	if current.ID != request.ExpectedActiveFeastID || !autoBuyerFeastExpectedExpiryMatches(current.ExpiresAt, request) {
		return request, source, feast, fmt.Errorf("%w: active feast changed", Intent.ErrPlanStale)
	}
	if current.ActiveAt(now) && current.ID != feast.ID {
		return request, source, feast, fmt.Errorf("%w: feast %d is already active", Intent.ErrPlanStale, current.ID)
	}
	if autoBuyerIntentRemaining(current.ExpiresAt, now) > int64(request.MinimumRemainingHours)*60*60 {
		return request, source, feast, fmt.Errorf("%w: %s already meets its configured floor", Intent.ErrPlanStale, feast.Name)
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, feast.Price)
	if !available {
		return request, source, feast, fmt.Errorf("%s balance is unavailable", feast.Price.Name)
	}
	effectiveCost, costErr := autoBuyerIntentFeastCost(input.State, feast, request.HistoryRefreshSec, now)
	if costErr != nil {
		return request, source, feast, costErr
	}
	if feast.Price.Premium && request.ExpectedBalanceBefore != nil && balance != *request.ExpectedBalanceBefore {
		return request, source, feast, fmt.Errorf("%w: %s balance changed before feast purchase", Intent.ErrPlanStale, feast.Price.Name)
	}
	if request.ExpectedEffectiveCost != nil && effectiveCost != *request.ExpectedEffectiveCost {
		return request, source, feast, fmt.Errorf("%w: effective feast cost changed before purchase", Intent.ErrPlanStale)
	}
	reserve := request.MinimumFoodReserve
	if feast.Price.Premium {
		reserve = request.MinimumRubyReserve
		if !request.AllowRubies || request.MaximumRubyCostPerPurchase < effectiveCost {
			return request, source, feast, fmt.Errorf("%s is not permitted within the configured ruby ceiling", feast.Name)
		}
	}
	if balance-reserve < effectiveCost {
		return request, source, feast, fmt.Errorf("%w: %s requires %d %s above reserve", Intent.ErrPlanStale, feast.Name, effectiveCost, feast.Price.Name)
	}
	return request, source, feast, nil
}

func autoBuyerIntentFeastCost(
	gameState State.GameState,
	feast GameData.AutoBuyerFeast,
	historyRefreshSec int,
	now time.Time,
) (int64, error) {
	if feast.Price.Premium {
		return feast.Price.Amount, nil
	}
	observedAt := gameState.Market.FeastCostReductionObservedAt
	if !autoBuyerIntentObservationFresh(observedAt, now, gameState.Session.ChangedAt, time.Duration(historyRefreshSec)*time.Second) {
		return 0, fmt.Errorf("%w: feast cost reduction is stale", Intent.ErrPlanStale)
	}
	percent := gameState.Market.FeastCostReductionPercent
	if percent < 0 || percent > 100 {
		return 0, fmt.Errorf("feast cost reduction is invalid")
	}
	return feast.EffectiveCost(percent), nil
}

func autoBuyerIntentObservationFresh(observedAt, now, sessionChangedAt time.Time, maxAge time.Duration) bool {
	return !observedAt.IsZero() && !observedAt.After(now) &&
		(sessionChangedAt.IsZero() || !observedAt.Before(sessionChangedAt)) &&
		maxAge > 0 && now.Sub(observedAt) < maxAge
}

func (application *Application) guardAutoBuyerPackagePurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	_, _, _, err = autoBuyerPackagePurchaseContext(input, arguments, time.Now().UTC(), true)
	return err
}

func (application *Application) verifyAutoBuyerPackagePurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	var request autoBuyerPackagePurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	product, found := input.GameData.AutoBuyerPackage(strings.TrimSpace(request.ShopID), int64(request.PackageID))
	if !found {
		return fmt.Errorf("package %d disappeared from the official Auto Buyer catalog", request.PackageID)
	}
	source, sourceErr := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if sourceErr != nil {
		return sourceErr
	}
	offers, _, found := input.State.ConstructionOffersFor(source.ID, source.KingdomID)
	if !found {
		return fmt.Errorf("package counters are unavailable for castle %d", source.ID)
	}
	purchased := offers[request.PackageID]
	if purchased < request.ExpectedPurchasedBefore+request.Amount {
		return fmt.Errorf("package %d purchase was not confirmed by the server counter", request.PackageID)
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, product.Price)
	if !available {
		return fmt.Errorf("%s balance is unavailable after package purchase", product.Price.Name)
	}
	reserve := request.MinimumBalanceReserve
	if product.Price.Premium {
		reserve = max(reserve, request.MinimumRubyReserve)
	}
	if balance < reserve {
		return fmt.Errorf("package purchase left %d %s below configured reserve %d", balance, product.Price.Name, reserve)
	}
	maximumCost := request.Amount * product.Price.Amount
	if request.ExpectedBalanceBefore > balance && request.ExpectedBalanceBefore-balance > maximumCost {
		return fmt.Errorf("package purchase consumed more %s than the official guarded cost", product.Price.Name)
	}
	return nil
}

func (application *Application) guardAutoBuyerSpecialistPurchase(ctx context.Context, arguments json.RawMessage) error {
	metadata := Outbound.MetadataFromContext(ctx)
	market := application.State.ReadOnlyView().Market
	allowPending := market.SpecialistPurchasePending && market.SpecialistPurchaseOperationID == strings.TrimSpace(metadata.OperationID) && market.SpecialistPurchaseResponseToken == strings.TrimSpace(metadata.ResponseToken)
	return application.validateAutoBuyerSpecialistDispatch(arguments, time.Now().UTC(), allowPending)
}

func (application *Application) verifyAutoBuyerSpecialistPurchase(ctx context.Context, arguments json.RawMessage) error {
	if err := application.reconcileAutoBuyerSpecialistPurchase(ctx, arguments); err != nil {
		return err
	}
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	specialist, found := GameData.AutoBuyerSpecialistByID(request.SpecialistID)
	if !found {
		return fmt.Errorf("specialist %d is no longer supported", request.SpecialistID)
	}
	evidence := input.State.Market.LatestSpecialistPurchase
	if input.State.Market.SpecialistPurchasePending || !evidence.ActivationConfirmed || evidence.SpecialistID != request.SpecialistID {
		return fmt.Errorf("%s renewal was not confirmed by the refreshed specialist timer", specialist.Name)
	}
	return nil
}

func (application *Application) validateAutoBuyerSpecialistDispatch(arguments json.RawMessage, now time.Time, allowMatchingPending bool) error {
	if application == nil || application.Configuration == nil || application.State == nil {
		return fmt.Errorf("Auto Buyer configuration is unavailable")
	}
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	configuration := application.Configuration.Snapshot()
	if !Automation.FeatureEnabledAt(configuration, "auto_buyer", now) {
		return fmt.Errorf("%w: Auto Buyer was disabled before specialist dispatch", Intent.ErrPlanStale)
	}
	state := application.State.ReadOnlyView()
	if !state.Session.LoggedIn || !state.Session.SocketReady || state.Session.ConnectionGeneration == 0 ||
		(request.ExpectedSessionGeneration > 0 && request.ExpectedSessionGeneration != state.Session.ConnectionGeneration) {
		return fmt.Errorf("%w: game session changed before specialist dispatch", Intent.ErrPlanStale)
	}
	if application.automationLocked() {
		return fmt.Errorf("%w: scheduler Bot Lock is active", Intent.ErrPlanStale)
	}
	if state.Automations["autoBuyer"].SafetyLock.Active(now) {
		return fmt.Errorf("%w: Auto Buyer lane safety lock is active", Intent.ErrPlanStale)
	}
	raw := configuration.Sections["automation.autoBuyer"]
	var current struct {
		Version            int   `json:"version"`
		MinimumRubyReserve int64 `json:"minimumRubyReserve"`
	}
	if json.Unmarshal(raw, &current) != nil || current.Version != 1 || current.MinimumRubyReserve != request.MinimumRubyReserve {
		return fmt.Errorf("%w: saved specialist settings changed before dispatch", Intent.ErrPlanStale)
	}
	matches := 0
	var document struct {
		Specialists []struct {
			Enabled                    bool  `json:"enabled"`
			ID                         int   `json:"id"`
			MinimumDays                int   `json:"minimumDays"`
			MaximumRubyCostPerPurchase int64 `json:"maximumRubyCostPerPurchase"`
		} `json:"specialists"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return fmt.Errorf("%w: decode saved specialist settings", Intent.ErrPlanStale)
	}
	for _, rule := range document.Specialists {
		if rule.ID == request.SpecialistID && rule.Enabled && rule.MinimumDays == request.MinimumDays && rule.MaximumRubyCostPerPurchase == request.MaximumRubyCostPerPurchase {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("%w: saved specialist settings changed before dispatch", Intent.ErrPlanStale)
	}
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	if allowMatchingPending {
		input.State.Market.SpecialistPurchasePending = false
	}
	_, _, err = autoBuyerSpecialistPurchaseContext(input, arguments, now)
	return err
}

func (application *Application) armAutoBuyerSpecialistPurchase(ctx context.Context, arguments json.RawMessage) error {
	if err := application.validateAutoBuyerSpecialistDispatch(arguments, time.Now().UTC(), false); err != nil {
		return err
	}
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	specialist, found := GameData.AutoBuyerSpecialistByID(request.SpecialistID)
	if !found {
		return fmt.Errorf("specialist is unsupported")
	}
	metadata := Outbound.MetadataFromContext(ctx)
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(state *State.GameState) ([]string, bool, error) {
		if state.Market.SpecialistPurchasePending {
			return nil, false, fmt.Errorf("%w: specialist purchase reconciliation is pending", Intent.ErrPlanStale)
		}
		now := time.Now().UTC()
		baseline := state.Market.Boosters[request.SpecialistID].ExpiresAt
		if baseline.Before(now) {
			baseline = now
		}
		state.Market.SpecialistPurchasePending = true
		state.Market.SpecialistPurchasePendingSince = now
		state.Market.SpecialistPurchaseExpectedID = request.SpecialistID
		state.Market.SpecialistPurchasePreviousExpiry = state.Market.Boosters[request.SpecialistID].ExpiresAt
		state.Market.SpecialistPurchaseMaximumExpiry = baseline.Add(time.Duration(specialist.DurationSec) * time.Second)
		state.Market.SpecialistPurchaseOperationID = strings.TrimSpace(metadata.OperationID)
		state.Market.SpecialistPurchaseResponseToken = strings.TrimSpace(metadata.ResponseToken)
		state.Market.SpecialistPurchaseResponseConfirmedAt = time.Time{}
		state.Market.SpecialistPurchaseResponseExpiresAt = time.Time{}
		state.Market.SpecialistPurchaseRubyResourceID = State.ResourceID(request.ExpectedRubyResourceID)
		state.Market.SpecialistPurchaseResponseRuby = 0
		state.Market.SpecialistPurchaseResponseRubyAt = time.Time{}
		state.Market.LatestSpecialistPurchase = State.SpecialistPurchaseEvidence{Outcome: "purchasing", SpecialistID: request.SpecialistID, Opcode: specialist.Opcode, AttemptedAt: now, UpdatedAt: now, MinimumDays: request.MinimumDays, ValidatedMaximumCost: specialist.ValidatedMaximumRubyCost, ConfiguredRubyCeiling: request.MaximumRubyCostPerPurchase, MinimumRubyReserve: request.MinimumRubyReserve, TimerBefore: state.Market.SpecialistPurchasePreviousExpiry, RubyBefore: request.ExpectedRubyBalance, RubyBeforeKnown: true, RubyBeforeObservedAt: request.ExpectedRubyObservedAt, DebitVerification: "unresolved"}
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	if err := application.saveStateEvent(ctx, event); err != nil {
		return err
	}
	return application.recordAutoBuyerSpecialistEvidence(ctx, strings.TrimSpace(metadata.OperationID))
}

func (application *Application) disarmAutoBuyerSpecialistPurchase(ctx context.Context, arguments json.RawMessage) error {
	return application.finishAutoBuyerSpecialistAttempt(ctx, arguments, "not-sent", "The command was definitively not sent")
}

func (application *Application) rejectAutoBuyerSpecialistPurchase(ctx context.Context, arguments json.RawMessage) error {
	return application.finishAutoBuyerSpecialistAttempt(ctx, arguments, "rejected", "The game definitively rejected the command")
}

func (application *Application) finishAutoBuyerSpecialistAttempt(ctx context.Context, arguments json.RawMessage, outcome string, detail string) error {
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(state *State.GameState) ([]string, bool, error) {
		market := &state.Market
		if !market.SpecialistPurchasePending || market.SpecialistPurchaseExpectedID != request.SpecialistID ||
			(market.SpecialistPurchaseOperationID != "" && market.SpecialistPurchaseOperationID != strings.TrimSpace(metadata.OperationID)) ||
			(market.SpecialistPurchaseResponseToken != "" && market.SpecialistPurchaseResponseToken != strings.TrimSpace(metadata.ResponseToken)) {
			return nil, false, nil
		}
		clearPendingSpecialistPurchase(market)
		market.LatestSpecialistPurchase.Outcome = outcome
		market.LatestSpecialistPurchase.UpdatedAt = time.Now().UTC()
		market.LatestSpecialistPurchase.Detail = detail
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	if err := application.saveStateEvent(ctx, event); err != nil {
		return err
	}
	return application.recordAutoBuyerSpecialistEvidence(ctx, strings.TrimSpace(metadata.OperationID))
}

func (application *Application) reconcileAutoBuyerSpecialistPurchase(ctx context.Context, _ json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	rubyResourceID, rubyFound := gameData.ResourceIDForJSONKey("C2")
	originOperationID := ""
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(state *State.GameState) ([]string, bool, error) {
		market := &state.Market
		if !market.SpecialistPurchasePending {
			return nil, false, nil
		}
		originOperationID = market.SpecialistPurchaseOperationID
		booster := market.Boosters[market.SpecialistPurchaseExpectedID]
		if state.Session.ConnectionGeneration == 0 || market.BoostersObservedGeneration != state.Session.ConnectionGeneration ||
			(!state.Session.ChangedAt.IsZero() && market.BoostersObservedAt.Before(state.Session.ChangedAt)) ||
			!market.BoostersObservedAt.After(market.SpecialistPurchasePendingSince) {
			return nil, false, nil
		}
		baseline := market.SpecialistPurchasePreviousExpiry
		if baseline.Before(market.SpecialistPurchasePendingSince) {
			baseline = market.SpecialistPurchasePendingSince
		}
		timerProgressed := !booster.Permanent && !booster.ExpiresAt.Before(baseline.Add(7*24*time.Hour-time.Second))
		if market.SpecialistPurchaseResponseConfirmedAt.IsZero() {
			if booster.ExpiresAt.IsZero() && market.BoostersObservedAt.After(market.SpecialistPurchaseMaximumExpiry) {
				market.LatestSpecialistPurchase.Outcome = "expired-unconfirmed"
				market.LatestSpecialistPurchase.UpdatedAt = time.Now().UTC()
				market.LatestSpecialistPurchase.Detail = "The bounded reconciliation window ended without a correlated purchase response or active timer"
				clearPendingSpecialistPurchase(market)
				return []string{"boosters", "market"}, true, nil
			}
			if timerProgressed && market.LatestSpecialistPurchase.Outcome != "unresolved" {
				market.LatestSpecialistPurchase.Outcome = "unresolved"
				market.LatestSpecialistPurchase.UpdatedAt = time.Now().UTC()
				market.LatestSpecialistPurchase.TimerAfter = booster.ExpiresAt
				market.LatestSpecialistPurchase.TimerAfterObservedAt = market.BoostersObservedAt
				market.LatestSpecialistPurchase.Detail = "A later authoritative timer increased, but no correlated response proves this operation caused it; spending remains latched"
				return []string{"boosters", "market"}, true, nil
			}
			return nil, false, nil
		}
		if market.BoostersObservedAt.Before(market.SpecialistPurchaseResponseConfirmedAt) || !timerProgressed ||
			booster.ExpiresAt.Before(market.SpecialistPurchaseResponseExpiresAt.Add(-time.Second)) ||
			booster.ExpiresAt.After(market.SpecialistPurchaseResponseExpiresAt.Add(time.Second)) {
			return nil, false, nil
		}
		evidence := &market.LatestSpecialistPurchase
		evidence.Outcome = "confirmed"
		evidence.ActivationConfirmed = true
		evidence.TimerAfter = booster.ExpiresAt
		evidence.TimerAfterObservedAt = market.BoostersObservedAt
		evidence.UpdatedAt = time.Now().UTC()
		evidence.Detail = "Authoritative booster expiry advanced by at least one seven-day activation"
		spendingVerified := false
		if rubyFound && rubyResourceID > 0 && market.SpecialistPurchaseResponseRubyAt.Equal(market.SpecialistPurchaseResponseConfirmedAt) {
			evidence.RubyAfter = market.SpecialistPurchaseResponseRuby
			evidence.RubyAfterKnown = true
			evidence.RubyAfterObservedAt = market.SpecialistPurchaseResponseRubyAt
			debit := evidence.RubyBefore - evidence.RubyAfter
			switch {
			case debit >= 0 && debit <= evidence.ValidatedMaximumCost && debit <= evidence.ConfiguredRubyCeiling && evidence.RubyAfter >= evidence.MinimumRubyReserve:
				evidence.DebitVerification = "command-local-observed"
				spendingVerified = true
			case debit > evidence.ValidatedMaximumCost || debit > evidence.ConfiguredRubyCeiling || evidence.RubyAfter < evidence.MinimumRubyReserve:
				evidence.DebitVerification = "discrepancy"
				evidence.Detail += "; command-local ruby evidence conflicts with the validated maximum, configured ceiling, or reserve"
			default:
				evidence.DebitVerification = "observed-unattributed"
			}
		}
		if !spendingVerified {
			evidence.Outcome = "activation-confirmed-spend-unresolved"
			evidence.Detail += "; spending evidence is unresolved, so automatic renewals remain latched"
			return []string{"boosters", "market"}, true, nil
		}
		clearPendingSpecialistPurchase(market)
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	if event.Revision > 0 {
		if err := application.saveStateEvent(ctx, event); err != nil {
			return err
		}
		return application.recordAutoBuyerSpecialistEvidence(ctx, originOperationID)
	}
	return nil
}

func (application *Application) recordAutoBuyerSpecialistEvidence(ctx context.Context, originOperationID string) error {
	evidence := application.State.ReadOnlyView().Market.LatestSpecialistPurchase
	return Intent.RecordOperationEvidence(ctx, "autoBuyer.specialist.purchase", struct {
		OriginOperationID string                           `json:"originOperationId,omitempty"`
		Purchase          State.SpecialistPurchaseEvidence `json:"purchase"`
	}{OriginOperationID: strings.TrimSpace(originOperationID), Purchase: evidence})
}

func clearPendingSpecialistPurchase(market *State.MarketState) {
	market.SpecialistPurchasePending = false
	market.SpecialistPurchaseOperationID = ""
	market.SpecialistPurchaseResponseToken = ""
	market.SpecialistPurchaseResponseConfirmedAt = time.Time{}
	market.SpecialistPurchaseResponseExpiresAt = time.Time{}
	market.SpecialistPurchaseRubyResourceID = 0
	market.SpecialistPurchaseResponseRuby = 0
	market.SpecialistPurchaseResponseRubyAt = time.Time{}
}

func (application *Application) armAutoBuyerFeastPurchase(ctx context.Context, arguments json.RawMessage) error {
	if err := application.validateAutoBuyerFeastDispatch(arguments, time.Now().UTC()); err != nil {
		return err
	}
	return application.setAutoBuyerFeastReconciliation(ctx, arguments, true)
}

func (application *Application) markAutoBuyerFeastReconciliation(ctx context.Context, arguments json.RawMessage) error {
	return application.setAutoBuyerFeastReconciliation(ctx, arguments, false)
}

func (application *Application) verifyAutoBuyerFeastReconciliation(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil {
		return fmt.Errorf("Auto Buyer state is unavailable")
	}
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(gameState *State.GameState) ([]string, bool, error) {
		market := &gameState.Market
		if !market.FeastPurchasePending || market.FeastPurchaseExpectedID != request.FeastID {
			return nil, false, nil
		}
		if !market.FeastPurchaseResponseConfirmedAt.IsZero() {
			return nil, false, fmt.Errorf("the correlated feast response still needs an authoritative matching timer snapshot")
		}
		refreshAge := time.Duration(request.HistoryRefreshSec) * time.Second
		feast := market.Feast
		baseline := market.FeastPurchasePreviousExpiresAt
		if baseline.IsZero() {
			baseline = market.FeastPurchasePendingSince
		}
		if !feast.FreshAt(now, gameState.Session.ChangedAt, refreshAge) ||
			!feast.ObservedAt.After(market.FeastPurchasePendingSince) || feast.ID != request.FeastID ||
			!State.FeastTimerProgressed(baseline, feast.ExpiresAt) {
			return nil, false, fmt.Errorf("the unresolved feast purchase has no authoritative increased timer")
		}
		source, sourceErr := autoBuyerIntentOwnedFeastCastle(*gameState, request.SourceCastleID, request.ExpectedSourceKingdomID)
		if sourceErr != nil {
			return nil, false, sourceErr
		}
		if !source.FoodBalanceObservedAt.After(market.FeastPurchasePendingSince) ||
			!autoBuyerIntentObservationFresh(source.FoodBalanceObservedAt, now, gameState.Session.ChangedAt, refreshAge) {
			return nil, false, fmt.Errorf("the unresolved feast purchase has no fresh charged-castle resource observation")
		}
		evidence := &market.LatestFeastPurchase
		evidence.Outcome = "reconciled-unverified"
		evidence.UpdatedAt = now
		evidence.ActivationConfirmed = false
		evidence.ConfirmedRemainingSec = feast.RemainingSec
		evidence.ConfirmedExpiresAt = feast.ExpiresAt
		evidence.Detail = "Timer progress and charged-castle resources were reconciled after a lost response; activation attribution remains unverified"
		if foodID, found := application.currentFoodResourceID(); found {
			if balance, exists := source.Resources[foodID]; exists {
				if amount, valid := autoBuyerIntentBalanceAmount(balance.Amount); valid {
					evidence.FoodAfter = amount
					evidence.FoodAfterKnown = true
					evidence.FoodAfterObservedAt = source.FoodBalanceObservedAt
				}
			}
		}
		clearAutoBuyerFeastPending(market)
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func clearAutoBuyerFeastPending(market *State.MarketState) {
	market.FeastPurchasePending = false
	market.FeastPurchaseExpectedID = 0
	market.FeastPurchasePendingSince = time.Time{}
	market.FeastPurchaseExpectedExpiresAt = time.Time{}
	market.FeastPurchasePreviousExpiresAt = time.Time{}
	market.FeastPurchaseOperationID = ""
	market.FeastPurchaseResponseToken = ""
	market.FeastPurchaseResponseConfirmedAt = time.Time{}
	market.FeastPurchaseResponseExpiresAt = time.Time{}
	market.FeastPurchaseInactiveObservedAt = time.Time{}
	market.FeastPurchaseInactiveResponseToken = ""
	market.FeastPurchaseInactiveGeneration = 0
}

func (application *Application) disarmAutoBuyerFeastPurchase(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil {
		return fmt.Errorf("Auto Buyer state is unavailable")
	}
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(gameState *State.GameState) ([]string, bool, error) {
		market := &gameState.Market
		if !market.FeastPurchasePending || market.FeastPurchaseExpectedID != request.FeastID {
			return nil, false, nil
		}
		metadata := Outbound.MetadataFromContext(ctx)
		if market.FeastPurchaseOperationID != "" &&
			market.FeastPurchaseOperationID != strings.TrimSpace(metadata.OperationID) {
			return nil, false, nil
		}
		if market.FeastPurchaseResponseToken != "" &&
			market.FeastPurchaseResponseToken != strings.TrimSpace(metadata.ResponseToken) {
			return nil, false, nil
		}
		market.FeastPurchasePending = false
		market.FeastPurchaseExpectedID = 0
		market.FeastPurchasePendingSince = time.Time{}
		market.FeastPurchaseExpectedExpiresAt = time.Time{}
		market.FeastPurchasePreviousExpiresAt = time.Time{}
		market.FeastPurchaseOperationID = ""
		market.FeastPurchaseResponseToken = ""
		market.FeastPurchaseResponseConfirmedAt = time.Time{}
		market.FeastPurchaseResponseExpiresAt = time.Time{}
		market.FeastPurchaseInactiveObservedAt = time.Time{}
		market.FeastPurchaseInactiveResponseToken = ""
		market.FeastPurchaseInactiveGeneration = 0
		if market.LatestFeastPurchase.AttemptedAt.Equal(request.AttemptAfter) {
			market.LatestFeastPurchase.Outcome = "not-sent"
			market.LatestFeastPurchase.UpdatedAt = time.Now().UTC()
			market.LatestFeastPurchase.Detail = "The purchase was not sent after a definitive transport failure"
		}
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) setAutoBuyerFeastReconciliation(ctx context.Context, arguments json.RawMessage, force bool) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("Auto Buyer state is unavailable")
	}
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.FeastID < 0 || request.AttemptAfter.IsZero() || force && request.FeastRefreshAfter.IsZero() {
		return fmt.Errorf("feast reconciliation boundary is invalid")
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	feast, found := gameData.AutoBuyerFeast(request.FeastID)
	if !found || feast.DurationSec <= 0 || feast.DurationSec > int64(math.MaxInt64)/int64(time.Second) {
		return fmt.Errorf("feast %d is unavailable for reconciliation", request.FeastID)
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(gameState *State.GameState) ([]string, bool, error) {
		market := &gameState.Market
		if market.FeastPurchasePending {
			if !force && market.LatestFeastPurchase.AttemptedAt.Equal(request.AttemptAfter) {
				market.LatestFeastPurchase.Outcome = "uncertain"
				market.LatestFeastPurchase.UpdatedAt = time.Now().UTC()
				market.LatestFeastPurchase.Detail = "Waiting for authoritative reconciliation before another purchase"
				return []string{"boosters", "market"}, true, nil
			}
			return nil, false, nil
		}
		if !force {
			return nil, false, nil
		}
		_, source, _, contextErr := autoBuyerFeastPurchaseContext(
			Intent.PlanningContext{State: *gameState, GameData: gameData}, arguments, time.Now().UTC(), true,
		)
		if contextErr != nil {
			return nil, false, contextErr
		}
		pendingSince := time.Now().UTC()
		baseline := pendingSince
		if market.Feast.ExpiresAt.After(baseline) {
			baseline = market.Feast.ExpiresAt
		}
		market.FeastPurchasePending = true
		market.FeastPurchaseInactiveObservedAt = time.Time{}
		market.FeastPurchaseInactiveResponseToken = ""
		market.FeastPurchaseInactiveGeneration = 0
		market.FeastPurchaseExpectedID = request.FeastID
		market.FeastPurchasePendingSince = pendingSince
		market.FeastPurchaseExpectedExpiresAt = baseline.Add(time.Duration(feast.DurationSec) * time.Second)
		market.FeastPurchasePreviousExpiresAt = market.Feast.ExpiresAt
		market.FeastPurchaseResponseConfirmedAt = time.Time{}
		market.FeastPurchaseResponseExpiresAt = time.Time{}
		evidence := State.FeastPurchaseEvidence{
			Outcome: "pending", FeastID: request.FeastID,
			ChargedCastleID: request.SourceCastleID, ChargedKingdomID: request.ExpectedSourceKingdomID,
			AttemptedAt: request.AttemptAfter, UpdatedAt: pendingSince,
			DebitVerification: "unverified",
		}
		if request.ExpectedEffectiveCost != nil {
			evidence.ExpectedEffectiveCost = *request.ExpectedEffectiveCost
		}
		if !feast.Price.Premium {
			foodID, found := gameData.ResourceIDForJSONKey("F")
			balance, exists := source.Resources[State.ResourceID(foodID)]
			amount, valid := autoBuyerIntentBalanceAmount(balance.Amount)
			if !found || foodID <= 0 || !exists || !valid || source.FoodBalanceObservedAt.IsZero() {
				return nil, false, fmt.Errorf("food balance is unavailable while arming feast reconciliation")
			}
			evidence.FoodBefore = amount
			evidence.FoodBeforeKnown = true
			evidence.FoodBeforeObservedAt = source.FoodBalanceObservedAt
		}
		market.LatestFeastPurchase = evidence
		if force {
			metadata := Outbound.MetadataFromContext(ctx)
			market.FeastPurchaseOperationID = strings.TrimSpace(metadata.OperationID)
			market.FeastPurchaseResponseToken = strings.TrimSpace(metadata.ResponseToken)
		}
		return []string{"boosters", "market"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) validateAutoBuyerFeastDispatch(arguments json.RawMessage, now time.Time) error {
	if application == nil || application.Configuration == nil || application.State == nil {
		return fmt.Errorf("Auto Buyer configuration is unavailable")
	}
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	configuration := application.Configuration.Snapshot()
	raw, found := configuration.Sections["automation.autoBuyer"]
	if !found {
		return fmt.Errorf("%w: Auto Buyer settings are unavailable", Intent.ErrPlanStale)
	}
	if err := Configuration.Validate("automation.autoBuyer", raw); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	var current struct {
		Version            int   `json:"version"`
		MinimumRubyReserve int64 `json:"minimumRubyReserve"`
		Feast              struct {
			Enabled                    bool  `json:"enabled"`
			FeastID                    int64 `json:"feastId"`
			MinimumRemainingHours      int   `json:"minimumRemainingHours"`
			MinimumFoodReserve         int64 `json:"minimumFoodReserve"`
			AllowRubies                bool  `json:"allowRubies"`
			MaximumRubyCostPerPurchase int64 `json:"maximumRubyCostPerPurchase"`
		} `json:"feast"`
	}
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("%w: decode current Auto Buyer settings: %v", Intent.ErrPlanStale, err)
	}
	state := application.State.ReadOnlyView()
	automation := state.Automations["autoBuyer"]
	if !state.Session.LoggedIn || !state.Session.SocketReady || state.Session.Generation == 0 {
		return fmt.Errorf("%w: game session is unavailable", Intent.ErrPlanStale)
	}
	if !Automation.FeatureEnabledAt(configuration, "auto_buyer", now) {
		return fmt.Errorf("%w: Auto Buyer was disabled before feast dispatch", Intent.ErrPlanStale)
	}
	if application.automationLocked() {
		return fmt.Errorf("%w: scheduler Bot Lock is active", Intent.ErrPlanStale)
	}
	if automation.SafetyLock.Active(now) {
		return fmt.Errorf("%w: Auto Buyer lane safety lock is active", Intent.ErrPlanStale)
	}
	if current.Version != 1 || !current.Feast.Enabled || current.Feast.FeastID != request.FeastID ||
		current.Feast.MinimumRemainingHours != request.MinimumRemainingHours ||
		current.Feast.MinimumFoodReserve != request.MinimumFoodReserve ||
		current.Feast.AllowRubies != request.AllowRubies ||
		current.Feast.MaximumRubyCostPerPurchase != request.MaximumRubyCostPerPurchase ||
		current.MinimumRubyReserve != request.MinimumRubyReserve {
		return fmt.Errorf("%w: saved feast settings changed before dispatch", Intent.ErrPlanStale)
	}
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	_, _, _, err = autoBuyerFeastPurchaseContext(input, arguments, now, true)
	return err
}

func (application *Application) verifyAutoBuyerFeastRefresh(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	return verifyAutoBuyerFeastRefreshContext(input, arguments)
}

func verifyAutoBuyerFeastRefreshContext(input Intent.PlanningContext, arguments json.RawMessage) error {
	var request autoBuyerBoostersRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if !request.FeastContext || request.FeastRefreshAfter.IsZero() {
		return fmt.Errorf("feast refresh verification is missing its authority boundary")
	}
	feastObservedAt := input.State.Market.Feast.ObservedAt
	costObservedAt := input.State.Market.FeastCostReductionObservedAt
	sessionChangedAt := input.State.Session.ChangedAt
	if feastObservedAt.Before(request.FeastRefreshAfter) ||
		!sessionChangedAt.IsZero() && feastObservedAt.Before(sessionChangedAt) {
		return fmt.Errorf("the game omitted feast status from the committed Auto Buyer refresh")
	}
	if costObservedAt.Before(request.FeastRefreshAfter) ||
		!sessionChangedAt.IsZero() && costObservedAt.Before(sessionChangedAt) {
		return fmt.Errorf("the game omitted feast cost reduction from the committed Auto Buyer refresh")
	}
	return nil
}

func (application *Application) verifyAutoBuyerFeastPurchase(ctx context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		_ = application.setAutoBuyerFeastReconciliation(ctx, arguments, false)
		return err
	}
	err = verifyAutoBuyerFeastPurchaseContext(input, arguments, time.Now().UTC())
	if err != nil {
		_ = application.setAutoBuyerFeastReconciliation(ctx, arguments, false)
		return err
	}
	return application.recordAutoBuyerFeastPostPurchase(ctx, arguments)
}

func verifyAutoBuyerFeastPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) error {
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.ExpectedBalanceBefore == nil || request.ExpectedEffectiveCost == nil || request.FeastRefreshAfter.IsZero() {
		return fmt.Errorf("feast verification is missing its guarded balance, cost, or refresh boundary")
	}
	feast, found := input.GameData.AutoBuyerFeast(request.FeastID)
	if !found {
		return fmt.Errorf("feast %d disappeared from the official catalog", request.FeastID)
	}
	current := input.State.Market.Feast
	refreshAge := time.Duration(request.HistoryRefreshSec) * time.Second
	if !current.FreshAt(now, input.State.Session.ChangedAt, refreshAge) {
		return fmt.Errorf("%s purchase was not confirmed by a current feast snapshot", feast.Name)
	}
	if input.State.Market.FeastLastPurchaseAt.Before(request.FeastRefreshAfter) ||
		!current.ObservedAt.After(input.State.Market.FeastLastPurchaseAt) {
		return fmt.Errorf("%s purchase was not confirmed by a committed direct feast response", feast.Name)
	}
	baseline := input.State.Market.FeastLastPurchaseAt
	expectedExpiry := request.ExpectedExpiresAt
	if expectedExpiry.IsZero() && request.ExpectedExpiresAtUnix > 0 {
		expectedExpiry = time.Unix(request.ExpectedExpiresAtUnix, 0).UTC()
	}
	if expectedExpiry.After(baseline) {
		baseline = expectedExpiry
	}
	if current.ID != feast.ID || !State.FeastTimerProgressed(baseline, current.ExpiresAt) {
		return fmt.Errorf("%s purchase was not confirmed by an increased refreshed feast timer", feast.Name)
	}
	evidence := input.State.Market.LatestFeastPurchase
	if evidence.FeastID != request.FeastID || evidence.ChargedCastleID != request.SourceCastleID ||
		evidence.ChargedKingdomID != request.ExpectedSourceKingdomID || evidence.Outcome != "confirmed" ||
		!evidence.ActivationConfirmed || evidence.ActivationConfirmedAt.Before(input.State.Market.FeastLastPurchaseAt) {
		return fmt.Errorf("%s purchase is missing correlated charged-source activation evidence", feast.Name)
	}
	source, sourceErr := autoBuyerIntentOwnedFeastCastle(input.State, request.SourceCastleID, request.ExpectedSourceKingdomID)
	if sourceErr != nil {
		return sourceErr
	}
	if !feast.Price.Premium &&
		(source.FoodBalanceObservedAt.Before(input.State.Market.FeastLastPurchaseAt) ||
			!autoBuyerIntentObservationFresh(source.FoodBalanceObservedAt, now, input.State.Session.ChangedAt, refreshAge)) {
		return fmt.Errorf("%s purchase balance was not confirmed by a post-purchase castle refresh", feast.Name)
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, feast.Price)
	if !available {
		return fmt.Errorf("%s balance is unavailable after feast purchase", feast.Price.Name)
	}
	return verifyAutoBuyerFeastBalance(request, feast, balance)
}

func (application *Application) recordAutoBuyerFeastPostPurchase(ctx context.Context, arguments json.RawMessage) error {
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(gameState *State.GameState) ([]string, bool, error) {
		evidence := &gameState.Market.LatestFeastPurchase
		if !evidence.ActivationConfirmed || evidence.FeastID != request.FeastID ||
			evidence.ChargedCastleID != request.SourceCastleID || evidence.ChargedKingdomID != request.ExpectedSourceKingdomID {
			return nil, false, fmt.Errorf("confirmed feast evidence no longer matches the charged source")
		}
		source, sourceErr := autoBuyerIntentOwnedFeastCastle(*gameState, request.SourceCastleID, request.ExpectedSourceKingdomID)
		if sourceErr != nil {
			return nil, false, sourceErr
		}
		foodID, found := application.currentFoodResourceID()
		if !found {
			return nil, false, fmt.Errorf("official food resource is unavailable")
		}
		balance, exists := source.Resources[foodID]
		amount, valid := autoBuyerIntentBalanceAmount(balance.Amount)
		if !exists || !valid {
			return nil, false, fmt.Errorf("food balance is unavailable after feast purchase")
		}
		evidence.FoodAfter = amount
		evidence.FoodAfterKnown = true
		evidence.FoodAfterObservedAt = source.FoodBalanceObservedAt
		evidence.DebitVerification = "unverified"
		evidence.UpdatedAt = time.Now().UTC()
		return []string{"market"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) currentFoodResourceID() (State.ResourceID, bool) {
	if application == nil || application.GameData == nil {
		return 0, false
	}
	store, ready := application.GameData.Current()
	if !ready || store == nil {
		return 0, false
	}
	id, found := store.ResourceIDForJSONKey("F")
	return State.ResourceID(id), found && id > 0
}

func verifyAutoBuyerFeastBalance(
	request autoBuyerFeastPurchaseRequest,
	feast GameData.AutoBuyerFeast,
	balance int64,
) error {
	if request.ExpectedBalanceBefore == nil || request.ExpectedEffectiveCost == nil {
		return fmt.Errorf("feast verification is missing its guarded balance or cost")
	}
	if !feast.Price.Premium {
		// Food changes continuously while the round trips run. Its reserve is a
		// dispatch-time guard against the immediately preceding DCL snapshot;
		// exact post-purchase debit comparisons would reject valid purchases.
		if balance < request.MinimumFoodReserve {
			return fmt.Errorf("%s purchase left %s below the configured reserve", feast.Name, feast.Price.Name)
		}
		return nil
	}
	reserve := request.MinimumFoodReserve
	if feast.Price.Premium {
		reserve = request.MinimumRubyReserve
		if !request.AllowRubies || *request.ExpectedEffectiveCost > request.MaximumRubyCostPerPurchase {
			return fmt.Errorf("%s purchase exceeded the configured ruby ceiling", feast.Name)
		}
	}
	if balance < reserve {
		return fmt.Errorf("%s purchase left %s below the configured reserve", feast.Name, feast.Price.Name)
	}
	if *request.ExpectedBalanceBefore > balance {
		debited := *request.ExpectedBalanceBefore - balance
		if feast.Price.Premium && debited > request.MaximumRubyCostPerPurchase {
			return fmt.Errorf("%s purchase exceeded the configured ruby ceiling", feast.Name)
		}
		if debited > *request.ExpectedEffectiveCost {
			return fmt.Errorf("%s purchase consumed more %s than the guarded effective cost", feast.Name, feast.Price.Name)
		}
	}
	return nil
}

func (application *Application) autoBuyerPlanningContext() (Intent.PlanningContext, error) {
	if application == nil || application.State == nil || application.GameData == nil {
		return Intent.PlanningContext{}, fmt.Errorf("Auto Buyer state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Intent.PlanningContext{}, fmt.Errorf("official game data is unavailable")
	}
	view := application.State.PlanningView()
	return Intent.PlanningContext{
		State: view.State, GameData: gameData, Partitions: view.Partitions, ProtocolContext: view.ProtocolContext,
	}, nil
}

func autoBuyerIntentSourceCastle(gameState State.GameState, castleID State.CastleID) (State.CastleState, error) {
	castle, found := gameState.Castles[castleID]
	if castleID <= 0 || !found || castle.KingdomID != 0 || castle.SlotType != 1 {
		return State.CastleState{}, fmt.Errorf("%w: castle %d is not the owned Great Empire main castle", Intent.ErrPlanStale, castleID)
	}
	return castle, nil
}

func autoBuyerIntentOwnedFeastCastle(
	gameState State.GameState,
	castleID State.CastleID,
	kingdomID State.KingdomID,
) (State.CastleState, error) {
	castle, found := gameState.Castles[castleID]
	if !found || castle.KingdomID != kingdomID || !GameData.AutoBuyerFeastCastleUsable(gameState, castleID, castle) {
		return State.CastleState{}, fmt.Errorf("%w: charged feast castle %d is no longer an owned usable castle", Intent.ErrPlanStale, castleID)
	}
	return castle, nil
}

func autoBuyerIntentPriceBalance(gameState State.GameState, source State.CastleState, price GameData.AutoBuyerPrice) (int64, bool) {
	switch price.Scope {
	case GameData.AutoBuyerPricePlayerResource:
		balance, found := gameState.Player.Resources[State.ResourceID(price.ResourceID)]
		amount, valid := autoBuyerIntentBalanceAmount(balance)
		return amount, found && price.ResourceID > 0 && valid
	case GameData.AutoBuyerPriceCastleResource:
		balance, found := source.Resources[State.ResourceID(price.ResourceID)]
		amount, valid := autoBuyerIntentBalanceAmount(balance.Amount)
		return amount, found && price.ResourceID > 0 && valid
	case GameData.AutoBuyerPriceCurrency:
		balance, found := gameState.Player.Currencies[State.CurrencyID(price.CurrencyID)]
		amount, valid := autoBuyerIntentBalanceAmount(balance)
		return amount, found && price.CurrencyID > 0 && valid
	default:
		return 0, false
	}
}

func autoBuyerIntentBalanceAmount(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(math.Floor(value)), true
}

func autoBuyerIntentLevelEligible(player State.PlayerState, minLevel, maxLevel, minLegend, maxLegend int64) bool {
	return GameData.PackageLevelEligible(player.Level, player.LegendLevel, minLevel, maxLevel, minLegend, maxLegend)
}

func autoBuyerIntentRemaining(expiresAt time.Time, now time.Time) int64 {
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return 0
	}
	return int64(expiresAt.Sub(now) / time.Second)
}

func autoBuyerIntentUnix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func autoBuyerIntentExpiryMatches(actual time.Time, expectedUnix int64) bool {
	if expectedUnix == 0 {
		return actual.IsZero()
	}
	if actual.IsZero() {
		return false
	}
	delta := actual.Unix() - expectedUnix
	return delta >= -5 && delta <= 5
}

func autoBuyerFeastExpectedExpiryMatches(actual time.Time, request autoBuyerFeastPurchaseRequest) bool {
	if !request.ExpectedExpiresAt.IsZero() {
		if actual.IsZero() {
			return false
		}
		delta := actual.Sub(request.ExpectedExpiresAt)
		return delta >= -time.Second && delta <= time.Second
	}
	return autoBuyerIntentExpiryMatches(actual, request.ExpectedExpiresAtUnix)
}
