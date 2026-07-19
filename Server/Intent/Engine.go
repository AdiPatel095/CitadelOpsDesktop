package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const sessionChangePollInterval = 25 * time.Millisecond
const wireCommitCleanupTimeout = 10 * time.Second

var ErrPlanStale = errors.New("intent plan became stale before dispatch")

type wireCommitCollectorContextKey struct{}

type commandDependencyReceiptContextKey struct{}

type effectPhaseCallbackContextKey struct{}

type effectPhaseCallback func(EffectPhase) error

type dispatchPermitContextKey struct{}

type dispatchPermit func(expectedRevision uint64) error

type commandDependencyReceipt struct {
	opcode string
	key    string
}

type wireCommitCollector struct {
	ingressIDs []uint64
}

func (collector *wireCommitCollector) add(ingressID uint64) {
	if collector != nil && ingressID > 0 {
		collector.ingressIDs = append(collector.ingressIDs, ingressID)
	}
}

func (collector *wireCommitCollector) flush(ctx context.Context, observer Observer) error {
	if collector == nil || len(collector.ingressIDs) == 0 {
		return nil
	}
	ingressIDs := append([]uint64(nil), collector.ingressIDs...)
	collector.ingressIDs = nil
	commitObserver, ok := observer.(CommitObserver)
	if !ok {
		return fmt.Errorf("committed wire response observer is unavailable")
	}
	cleanupContext := context.Background()
	if ctx != nil {
		cleanupContext = context.WithoutCancel(ctx)
	}
	cleanupContext, cancel := context.WithTimeout(cleanupContext, wireCommitCleanupTimeout)
	defer cancel()
	for index, ingressID := range ingressIDs {
		committed, err := commitObserver.WaitCommitted(cleanupContext, ingressID)
		if err != nil {
			for _, remainingID := range ingressIDs[index+1:] {
				commitObserver.ForgetCommitted(remainingID)
			}
			return err
		}
		if committed.ReduceError != "" {
			for _, remainingID := range ingressIDs[index+1:] {
				commitObserver.ForgetCommitted(remainingID)
			}
			return fmt.Errorf("reduce response: %s", committed.ReduceError)
		}
	}
	return nil
}

type Engine struct {
	registry  *Registry
	state     StateReader
	gameData  GameDataProvider
	sender    Sender
	observer  Observer
	claims    *claimManager
	admission *admissionManager

	mu              sync.RWMutex
	actions         map[string]Action
	resolvers       map[string]StepResolver
	dependencies    map[string]CommandDependencyResolver
	executionGate   ExecutionGate
	admissionWeight AdmissionWeightProvider
	operationStore  OperationStore
	active          map[string]context.CancelFunc
	operations      map[string]Receipt
	requestHashes   map[string]string
	operationOrder  []string
	operationIndex  map[string]struct{}
	subscribers     map[uint64]chan Receipt
	eventSequence   uint64
	persistenceErr  error
	nextID          atomic.Uint64
	nextSubID       atomic.Uint64
	nextResponseID  atomic.Uint64
}

func NewEngine(registry *Registry, state StateReader, gameData GameDataProvider, sender Sender, observer Observer) *Engine {
	if registry == nil {
		registry = NewRegistry()
	}
	availability := func(class AdmissionClass) time.Time {
		if class != AdmissionAttackLaunch {
			return time.Time{}
		}
		provider, ok := sender.(LaneAvailability)
		if !ok || provider == nil {
			return time.Time{}
		}
		return provider.NextAllowed(Outbound.LaneAttackLaunch)
	}
	return &Engine{
		registry: registry, state: state, gameData: gameData, sender: sender, observer: observer,
		claims: newClaimManager(), admission: newAdmissionManager(availability), actions: map[string]Action{}, resolvers: map[string]StepResolver{},
		dependencies: map[string]CommandDependencyResolver{}, active: map[string]context.CancelFunc{},
		operations: map[string]Receipt{}, requestHashes: map[string]string{}, operationIndex: map[string]struct{}{},
		subscribers: map[uint64]chan Receipt{},
	}
}

func (engine *Engine) Registry() *Registry {
	return engine.registry
}

func (engine *Engine) RegisterAction(name string, action Action) error {
	name = strings.TrimSpace(name)
	if name == "" || action == nil {
		return fmt.Errorf("action name and implementation are required")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, exists := engine.actions[name]; exists {
		return fmt.Errorf("action %q is already registered", name)
	}
	engine.actions[name] = action
	return nil
}

func (engine *Engine) RegisterStepResolver(name string, resolver StepResolver) error {
	name = strings.TrimSpace(name)
	if name == "" || resolver == nil {
		return fmt.Errorf("step resolver name and implementation are required")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, exists := engine.resolvers[name]; exists {
		return fmt.Errorf("step resolver %q is already registered", name)
	}
	engine.resolvers[name] = resolver
	return nil
}

func (engine *Engine) RegisterCommandDependencies(opcode string, resolver CommandDependencyResolver) error {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || resolver == nil {
		return fmt.Errorf("command opcode and dependency resolver are required")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, exists := engine.dependencies[opcode]; exists {
		return fmt.Errorf("command dependencies for %q are already registered", opcode)
	}
	engine.dependencies[opcode] = resolver
	return nil
}

func (engine *Engine) SetExecutionGate(gate ExecutionGate) {
	engine.mu.Lock()
	engine.executionGate = gate
	engine.mu.Unlock()
}

func (engine *Engine) SetAdmissionWeightProvider(provider AdmissionWeightProvider) {
	engine.mu.Lock()
	engine.admissionWeight = provider
	engine.mu.Unlock()
}

func (engine *Engine) SetOperationStore(ctx context.Context, store OperationStore) error {
	if engine == nil || store == nil {
		return fmt.Errorf("operation store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	recovered, err := store.Recover(ctx)
	if err != nil {
		return err
	}
	recent, err := store.Recent(ctx, operationHistoryLimit)
	if err != nil {
		return err
	}
	engine.mu.Lock()
	if engine.operationStore != nil {
		engine.mu.Unlock()
		return fmt.Errorf("operation store is already configured")
	}
	engine.operationStore = store
	for index := len(recent) - 1; index >= 0; index-- {
		operation := recent[index]
		engine.cacheOperationLocked(operation.Receipt, operation.RequestHash)
	}
	for _, operation := range recovered {
		engine.cacheOperationLocked(operation.Receipt, operation.RequestHash)
	}
	engine.mu.Unlock()
	return nil
}

func (engine *Engine) Submit(ctx context.Context, request Request) Receipt {
	if ctx == nil {
		ctx = context.Background()
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		request.ID = fmt.Sprintf("op-%d-%d", time.Now().UTC().UnixMilli(), engine.nextID.Add(1))
	}
	request.Actor = strings.TrimSpace(request.Actor)
	if request.Actor == "" {
		request.Actor = "api"
	}
	if len(request.Arguments) == 0 {
		request.Arguments = json.RawMessage(`{}`)
	}
	priority, priorityErr := Outbound.ResolvePriority(request.Actor, request.Priority)
	receipt := Receipt{
		ID: request.ID, Intent: request.Name, Actor: request.Actor, Priority: priority,
		Status: StatusPlanning, Phase: EffectPhaseAccepted, SubmittedAt: time.Now().UTC(),
	}
	request.Priority = priority
	reserved, created, reserveErr := engine.reserveOperation(ctx, requestFingerprint(request), receipt)
	if reserveErr != nil {
		now := time.Now().UTC()
		receipt.Status = StatusFailed
		receipt.Phase = EffectPhaseCompleted
		receipt.Error = reserveErr.Error()
		receipt.CompletedAt = &now
		return receipt
	}
	if !created {
		return reserved
	}
	executionContext, cancel := context.WithCancel(ctx)
	if !engine.registerActive(request.ID, cancel) {
		cancel()
		if existing, ok := engine.Operation(request.ID); ok {
			return existing
		}
		return reserved
	}
	executionContext = Outbound.WithMetadata(executionContext, Outbound.Metadata{
		OperationID: request.ID, Intent: request.Name, Actor: request.Actor, SubmittedAt: receipt.SubmittedAt, Priority: priority,
	})
	defer func() {
		cancel()
		engine.unregisterActive(request.ID)
	}()
	if priorityErr != nil {
		return engine.fail(receipt, priorityErr)
	}

	definition, exists := engine.registry.Definition(request.Name)
	if !exists {
		return engine.fail(receipt, fmt.Errorf("unknown intent %q", request.Name))
	}
	if engine.state == nil {
		return engine.fail(receipt, fmt.Errorf("state store is unavailable"))
	}
	completedSteps := map[string]int{}
	var checkpointPlan *Plan
	var admissionRelease func()
	var admitted *Admission
	releaseAdmission := func() {
		if admissionRelease != nil {
			admissionRelease()
		}
		admissionRelease = nil
		admitted = nil
	}
	defer releaseAdmission()
	expectedRevisionAccepted := false
	firstPlan := true
	operationConnectionGeneration := uint64(0)
	for {
		if err := executionContext.Err(); err != nil {
			return engine.fail(receipt, err)
		}
		planningInput := engine.planningContext()
		plannedFrom := planningInput.State
		if operationConnectionGeneration == 0 && sessionHasAuthoritativeBaseline(plannedFrom.Session) {
			operationConnectionGeneration = plannedFrom.Session.ConnectionGeneration
		}
		if !expectedRevisionAccepted && request.ExpectedRevision != nil && plannedFrom.Revision != *request.ExpectedRevision {
			return engine.fail(receipt, fmt.Errorf(
				"state revision changed: expected %d, current %d",
				*request.ExpectedRevision, plannedFrom.Revision,
			))
		}
		useCheckpoint := checkpointPlan != nil && len(completedSteps) > 0
		var plan Plan
		if useCheckpoint {
			plan = *checkpointPlan
			plan.StateRevision = plannedFrom.Revision
		} else {
			planned, err := definition.Planner(
				executionContext,
				planningInput,
				request.Arguments,
			)
			if err != nil {
				return engine.fail(receipt, err)
			}
			plan, err = finalizePlan(definition, planningInput, request.Arguments, planned)
			if err != nil {
				return engine.fail(receipt, err)
			}
		}
		receipt.Plan = &plan
		if firstPlan && request.DryRun {
			receipt.Status = StatusPlanned
			receipt.Phase = EffectPhaseCompleted
			now := time.Now().UTC()
			receipt.CompletedAt = &now
			engine.update(receipt)
			return receipt
		}
		firstPlan = false
		if err := engine.awaitExecutionGate(executionContext, request, plan, ExecutionBeforeClaims); err != nil {
			if errors.Is(err, Outbound.ErrAutomationLocked) {
				receipt = engine.pause(receipt)
				continue
			}
			return engine.fail(receipt, err)
		}
		admissionChanged := false
		if !sameAdmission(admitted, plan.Admission) {
			releaseAdmission()
			admissionChanged = true
		}
		if plan.Admission != nil && admitted == nil {
			receipt.Status = StatusQueued
			receipt.Phase = EffectPhasePlanned
			receipt.Error = ""
			engine.update(receipt)
			weight := engine.resolveAdmissionWeight(request, *plan.Admission)
			release, err := engine.admission.acquire(executionContext, admissionRequest{
				operationID: request.ID, actor: request.Actor, priority: request.Priority,
				weight: weight, submittedAt: receipt.SubmittedAt, admission: *plan.Admission,
			})
			if err != nil {
				return engine.fail(receipt, err)
			}
			admissionRelease = release
			copy := *plan.Admission
			admitted = &copy
			admissionChanged = true
		}

		claimContext := executionContext
		cancelClaimWait := func() {}
		if admitted != nil {
			claimContext, cancelClaimWait = context.WithTimeout(executionContext, admissionClaimTimeout)
		}
		release, claimsWaited, err := engine.claims.acquireResources(claimContext, plan.Resources, request.Priority)
		cancelClaimWait()
		if err != nil {
			if admitted != nil && errors.Is(err, context.DeadlineExceeded) && executionContext.Err() == nil {
				releaseAdmission()
				continue
			}
			return engine.fail(receipt, err)
		}
		// Reuse the planning snapshot only when claims, admission, checkpoints,
		// and the state revision could not have made the plan stale.
		revalidate := useCheckpoint || admissionChanged || claimsWaited || engine.state.Revision() != plannedFrom.Revision
		current := plannedFrom
		currentInput := planningInput
		if revalidate {
			currentInput = engine.planningContext()
			current = currentInput.State
		}
		if !expectedRevisionAccepted && request.ExpectedRevision != nil && current.Revision != *request.ExpectedRevision {
			release()
			return engine.fail(receipt, fmt.Errorf(
				"state revision changed while waiting for claims: expected %d, current %d",
				*request.ExpectedRevision, current.Revision,
			))
		}
		if useCheckpoint {
			plan.StateRevision = current.Revision
		} else if revalidate && definition.ReadSet != nil && !admissionChanged && !claimsWaited &&
			currentInput.Partitions.Available() && currentInput.Partitions.Current(plan.Dependencies) {
			plan.StateRevision = current.Revision
		} else if revalidate {
			revalidated, err := definition.Planner(
				executionContext,
				currentInput,
				request.Arguments,
			)
			if err != nil {
				release()
				return engine.fail(receipt, err)
			}
			revalidated, err = finalizePlan(definition, currentInput, request.Arguments, revalidated)
			if err != nil {
				release()
				return engine.fail(receipt, err)
			}
			if !sameStrings(plan.Claims, revalidated.Claims) ||
				!sameResourceKeys(plan.Resources, revalidated.Resources) ||
				!sameAdmission(plan.Admission, revalidated.Admission) {
				release()
				releaseAdmission()
				continue
			}
			plan = revalidated
		}
		expectedRevisionAccepted = true
		checkpoint := plan
		checkpointPlan = &checkpoint
		receipt.Plan = &plan
		if receipt.StartedAt == nil {
			started := time.Now().UTC()
			receipt.StartedAt = &started
		}
		receipt.Status = StatusRunning
		receipt.Phase = EffectPhasePlanned
		receipt.Error = ""
		if err := engine.update(receipt); err != nil {
			release()
			return engine.persistenceFailure(receipt, fmt.Errorf("persist operation before execution: %w", err))
		}

		currentRevision := current.Revision
		completedThisAttempt := map[string]int{}
		yielded := false
		replan := false
		wireCommits := &wireCommitCollector{}
		attemptContext := context.WithValue(executionContext, wireCommitCollectorContextKey{}, wireCommits)
		attemptContext = context.WithValue(attemptContext, effectPhaseCallbackContextKey{}, effectPhaseCallback(func(phase EffectPhase) error {
			receipt.Phase = phase
			if phase == EffectPhaseDispatching {
				receipt.Attempt++
			}
			return engine.update(receipt)
		}))
		dispatches := 0
		expectedProtocolContext := currentInput.ProtocolContext
		expectedWorldID, expectedPlayerID := State.BoundAccount(currentInput.State)
		expectedCatalogVersion := plan.CatalogVersion
		attemptContext = context.WithValue(attemptContext, dispatchPermitContextKey{}, dispatchPermit(func(expectedRevision uint64) error {
			view := engine.planningContext()
			if dispatches == 0 {
				if len(plan.Dependencies) > 0 && view.Partitions.Available() {
					if !view.Partitions.Current(plan.Dependencies) {
						return fmt.Errorf("%w: a declared state partition changed", ErrPlanStale)
					}
				} else if view.State.Revision != expectedRevision {
					return fmt.Errorf("%w: state revision changed from %d to %d", ErrPlanStale, expectedRevision, view.State.Revision)
				}
			}
			if expectedProtocolContext.SessionGeneration > 0 &&
				view.ProtocolContext.SessionGeneration != expectedProtocolContext.SessionGeneration {
				return fmt.Errorf("%w: session generation changed", ErrPlanStale)
			}
			if expectedProtocolContext.ConnectionGeneration > 0 &&
				view.ProtocolContext.ConnectionGeneration != expectedProtocolContext.ConnectionGeneration {
				return fmt.Errorf("%w: connection generation changed", ErrPlanStale)
			}
			currentWorldID, currentPlayerID := State.BoundAccount(view.State)
			if expectedWorldID != "" && !strings.EqualFold(expectedWorldID, currentWorldID) {
				return fmt.Errorf("%w: bound game world changed", ErrPlanStale)
			}
			if expectedPlayerID > 0 && currentPlayerID != expectedPlayerID {
				return fmt.Errorf("%w: bound game account changed", ErrPlanStale)
			}
			if expectedCatalogVersion != "" && currentCatalogVersion(view) != expectedCatalogVersion {
				return fmt.Errorf("%w: official game-data catalog changed", ErrPlanStale)
			}
			if dispatches == 0 && planUsesFocus(plan.Resources) &&
				(view.ProtocolContext.FocusEpoch != expectedProtocolContext.FocusEpoch ||
					view.ProtocolContext.FocusedCastleID != expectedProtocolContext.FocusedCastleID) {
				return fmt.Errorf("%w: focused castle context changed", ErrPlanStale)
			}
			dispatches++
			return nil
		}))
		flushWireCommits := func() error {
			return wireCommits.flush(executionContext, engine.observer)
		}
		for _, step := range plan.Steps {
			resumeKey := stepResumeKey(step)
			if step.ResumePolicy != ResumeRebuild && completedThisAttempt[resumeKey] < completedSteps[resumeKey] {
				completedThisAttempt[resumeKey]++
				continue
			}
			if err := engine.awaitExecutionGate(executionContext, request, plan, ExecutionBeforeStep); err != nil {
				if flushErr := flushWireCommits(); flushErr != nil {
					release()
					return engine.failAfterProgress(receipt, fmt.Errorf("commit acknowledged response before yielding: %w", flushErr), completedSteps)
				}
				if errors.Is(err, Outbound.ErrAutomationLocked) {
					yielded = true
					break
				}
				release()
				return engine.failAfterProgress(receipt, err, completedSteps)
			}
			if step.ResponseBarrier != ResponseBarrierWire && step.ResponseBarrier != ResponseBarrierWireThenCommitted {
				if err := flushWireCommits(); err != nil {
					release()
					return engine.failAfterProgress(receipt, fmt.Errorf("commit acknowledged response before state-dependent step: %w", err), completedSteps)
				}
				currentRevision = engine.state.Revision()
			}
			stepContext := attemptContext
			if operationConnectionGeneration == 0 {
				session := engine.state.Session()
				if sessionHasAuthoritativeBaseline(session) {
					operationConnectionGeneration = session.ConnectionGeneration
				}
			}
			if operationConnectionGeneration > 0 {
				metadata := Outbound.MetadataFromContext(attemptContext)
				metadata.ConnectionGeneration = operationConnectionGeneration
				stepContext = Outbound.WithMetadata(attemptContext, metadata)
			}
			exchange, err := engine.executeStep(stepContext, currentRevision, step)
			if exchange != nil {
				receipt.Exchanges = append(receipt.Exchanges, *exchange)
				engine.update(receipt)
			}
			if err != nil {
				if flushErr := flushWireCommits(); flushErr != nil {
					err = errors.Join(err, fmt.Errorf("commit earlier acknowledged response: %w", flushErr))
				}
				if errors.Is(err, Outbound.ErrAutomationLocked) {
					yielded = true
					break
				}
				if errors.Is(err, ErrPlanStale) && !completedAnyStep(completedSteps) {
					replan = true
					break
				}
				release()
				return engine.failAfterProgress(receipt, fmt.Errorf("%s: %w", stepLabel(step), err), completedSteps)
			}
			if step.ResumePolicy != ResumeRebuild {
				completedSteps[resumeKey]++
				completedThisAttempt[resumeKey]++
			}
			if step.ResponseBarrier != ResponseBarrierWire {
				if err := flushWireCommits(); err != nil {
					release()
					return engine.failAfterProgress(receipt, fmt.Errorf("commit acknowledged response: %w", err), completedSteps)
				}
				currentRevision = engine.state.Revision()
			}
		}
		if err := flushWireCommits(); err != nil {
			release()
			return engine.failAfterProgress(receipt, fmt.Errorf("commit acknowledged response before completion: %w", err), completedSteps)
		}
		release()
		if replan {
			releaseAdmission()
			checkpointPlan = nil
			receipt.Status = StatusPlanning
			receipt.Phase = EffectPhaseAccepted
			receipt.Error = ""
			if err := engine.update(receipt); err != nil {
				return engine.persistenceFailure(receipt, fmt.Errorf("persist replanning operation: %w", err))
			}
			continue
		}
		if yielded {
			releaseAdmission()
			receipt = engine.pause(receipt)
			continue
		}
		releaseAdmission()
		receipt.Status = StatusSucceeded
		receipt.Phase = EffectPhaseCompleted
		receipt.Error = ""
		completed := time.Now().UTC()
		receipt.CompletedAt = &completed
		if err := engine.update(receipt); err != nil {
			receipt.Status = StatusIndeterminate
			if receipt.Plan != nil && receipt.Plan.Effect == EffectRead {
				receipt.Status = StatusFailed
			}
			receipt.Phase = EffectPhaseReconciliationRequired
			receipt.Error = fmt.Sprintf("persist completed operation: %v", err)
			engine.publish(receipt)
		}
		return receipt
	}
}

func completedAnyStep(completedSteps map[string]int) bool {
	for _, count := range completedSteps {
		if count > 0 {
			return true
		}
	}
	return false
}

func planUsesFocus(resources []ResourceKey) bool {
	for _, resource := range resources {
		if resource.Scope == ResourceScopeSession && resource.Capability == "session" && resource.ResourceKind == "focus" {
			return true
		}
	}
	return false
}

func (engine *Engine) resolveAdmissionWeight(request Request, admission Admission) int {
	engine.mu.RLock()
	provider := engine.admissionWeight
	engine.mu.RUnlock()
	if provider == nil {
		return defaultAdmissionWeight
	}
	return normalizeAdmissionWeight(provider(request, admission))
}

func (engine *Engine) Cancel(id string) bool {
	id = strings.TrimSpace(id)
	engine.mu.RLock()
	cancel := engine.active[id]
	engine.mu.RUnlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (engine *Engine) registerActive(id string, cancel context.CancelFunc) bool {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, exists := engine.active[id]; exists {
		return false
	}
	engine.active[id] = cancel
	return true
}

func (engine *Engine) unregisterActive(id string) {
	engine.mu.Lock()
	delete(engine.active, id)
	engine.mu.Unlock()
}

func (engine *Engine) awaitExecutionGate(ctx context.Context, request Request, plan Plan, point ExecutionPoint) error {
	engine.mu.RLock()
	gate := engine.executionGate
	engine.mu.RUnlock()
	if gate == nil {
		return nil
	}
	return gate(ctx, request, plan, point)
}

func (engine *Engine) pause(receipt Receipt) Receipt {
	receipt.Status = StatusPaused
	receipt.Error = ""
	receipt.CompletedAt = nil
	engine.update(receipt)
	return receipt
}

func (engine *Engine) persistenceFailure(receipt Receipt, err error) Receipt {
	receipt.Status = StatusFailed
	receipt.Phase = EffectPhaseCompleted
	receipt.Error = err.Error()
	now := time.Now().UTC()
	receipt.CompletedAt = &now
	engine.publish(receipt)
	return receipt
}

func stepResumeKey(step Step) string {
	return strings.Join([]string{
		step.Name,
		step.Action,
		string(step.ActionArguments),
		step.Resolver,
		string(step.ResolverArguments),
		step.Opcode,
		string(step.Payload),
		step.AwaitOpcode,
		strings.Join(step.AwaitOpcodes, ","),
		string(step.ResponseBarrier),
		fmt.Sprint(step.TimeoutMillis),
		fmt.Sprint(step.DelayMillis),
		fmt.Sprint(step.SuccessCodes),
		fmt.Sprint(step.CaptureResponse),
		func() string {
			if step.CommandDependencies == nil {
				return ""
			}
			return step.CommandDependencies.Opcode + "\x1f" + string(step.CommandDependencies.Payload)
		}(),
		step.Command.Namespace,
		step.Command.Opcode,
		step.Command.Sequence,
		step.Command.Route,
		string(step.Command.Payload),
		fmt.Sprint(step.Command.Bare),
		fmt.Sprint(step.Command.OmitNamespace),
	}, "\x00")
}

func (engine *Engine) Operation(id string) (Receipt, bool) {
	engine.mu.RLock()
	receipt, ok := engine.operations[id]
	store := engine.operationStore
	engine.mu.RUnlock()
	if ok || store == nil {
		return receipt, ok
	}
	operation, ok, err := store.Get(context.Background(), id)
	engine.recordPersistenceFailure(err)
	if err != nil || !ok {
		return Receipt{}, false
	}
	engine.mu.Lock()
	engine.cacheOperationLocked(operation.Receipt, operation.RequestHash)
	engine.mu.Unlock()
	return operation.Receipt, true
}

func (engine *Engine) RecentOperations(ctx context.Context, limit int) ([]Receipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > operationHistoryLimit {
		limit = operationHistoryLimit
	}
	engine.mu.RLock()
	store := engine.operationStore
	if store == nil {
		ids := append([]string(nil), engine.operationOrder...)
		receipts := make(map[string]Receipt, len(engine.operations))
		for id, receipt := range engine.operations {
			receipts[id] = receipt
		}
		engine.mu.RUnlock()
		if len(ids) > limit {
			ids = ids[len(ids)-limit:]
		}
		out := make([]Receipt, 0, len(ids))
		for index := len(ids) - 1; index >= 0; index-- {
			if receipt, ok := receipts[ids[index]]; ok {
				out = append(out, receipt)
			}
		}
		return out, nil
	}
	engine.mu.RUnlock()
	operations, err := store.Recent(ctx, limit)
	engine.recordPersistenceFailure(err)
	if err != nil {
		return nil, err
	}
	out := make([]Receipt, 0, len(operations))
	for _, operation := range operations {
		out = append(out, operation.Receipt)
	}
	return out, nil
}

func (engine *Engine) PersistenceError() error {
	if engine == nil {
		return nil
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.persistenceErr
}

func (engine *Engine) recordPersistenceResult(err error) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	engine.persistenceErr = err
	engine.mu.Unlock()
}

func (engine *Engine) recordPersistenceFailure(err error) {
	if engine == nil || err == nil {
		return
	}
	engine.mu.Lock()
	engine.persistenceErr = err
	engine.mu.Unlock()
}

func (engine *Engine) reserveOperation(
	ctx context.Context,
	requestHash string,
	receipt Receipt,
) (Receipt, bool, error) {
	engine.mu.RLock()
	store := engine.operationStore
	engine.mu.RUnlock()
	if store != nil {
		reserved, created, err := store.Reserve(ctx, requestHash, receipt)
		engine.recordPersistenceResult(err)
		if err != nil {
			return Receipt{}, false, err
		}
		engine.mu.Lock()
		engine.cacheOperationLocked(reserved, requestHash)
		if created {
			engine.publishLocked(reserved)
		}
		engine.mu.Unlock()
		return reserved, created, nil
	}
	engine.mu.Lock()
	if existing, exists := engine.operations[receipt.ID]; exists {
		existingHash := engine.requestHashes[receipt.ID]
		engine.mu.Unlock()
		if existingHash != requestHash {
			return Receipt{}, false, fmt.Errorf("%w: %s", ErrIdempotencyConflict, receipt.ID)
		}
		return existing, false, nil
	}
	engine.cacheOperationLocked(receipt, requestHash)
	engine.publishLocked(receipt)
	engine.mu.Unlock()
	return receipt, true, nil
}

func (engine *Engine) Subscribe(buffer int) (<-chan Receipt, func()) {
	if buffer < 1 {
		buffer = 1
	}
	id := engine.nextSubID.Add(1)
	channel := make(chan Receipt, buffer)
	engine.mu.Lock()
	engine.subscribers[id] = channel
	engine.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			engine.mu.Lock()
			delete(engine.subscribers, id)
			engine.mu.Unlock()
		})
	}
}

func (engine *Engine) executeStep(ctx context.Context, afterRevision uint64, step Step) (*CommandExchange, error) {
	if step.DelayMillis > 0 {
		timer := time.NewTimer(time.Duration(step.DelayMillis) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		if step.Action == "" && step.Resolver == "" && step.Opcode == "" && step.Command.Opcode == "" {
			return nil, nil
		}
	}
	if step.Resolver != "" {
		engine.mu.RLock()
		resolver := engine.resolvers[step.Resolver]
		engine.mu.RUnlock()
		if resolver == nil {
			return nil, fmt.Errorf("step resolver %q is not registered", step.Resolver)
		}
		resolvedContext := ctx
		if dependency := step.CommandDependencies; dependency != nil {
			dependencyStep := Step{
				Name: step.Name, Opcode: dependency.Opcode, Payload: dependency.Payload,
				Command: Protocol.Command{Opcode: dependency.Opcode, Payload: dependency.Payload},
			}
			var key string
			var err error
			afterRevision, key, err = engine.executeCommandDependencies(ctx, afterRevision, dependencyStep)
			if err != nil {
				return nil, err
			}
			resolvedContext = context.WithValue(ctx, commandDependencyReceiptContextKey{}, commandDependencyReceipt{
				opcode: strings.ToLower(strings.TrimSpace(dependency.Opcode)), key: key,
			})
		}
		planningInput := engine.planningContext()
		current := planningInput.State
		resolved, err := resolver(ctx, planningInput, step.ResolverArguments)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resolved.Resolver) != "" || strings.TrimSpace(resolved.Action) != "" {
			return nil, fmt.Errorf("step resolver %q must return one concrete command step", step.Resolver)
		}
		resolvedPlan := normalizePlan(Definition{}, current.Revision, Plan{Steps: []Step{resolved}})
		resolved = resolvedPlan.Steps[0]
		if !sameStrings(stepAwaitOpcodes(step), stepAwaitOpcodes(resolved)) {
			return nil, fmt.Errorf("step resolver %q changed its declared response claims", step.Resolver)
		}
		return engine.executeStep(resolvedContext, current.Revision, resolved)
	}
	if step.Action != "" {
		engine.mu.RLock()
		action := engine.actions[step.Action]
		engine.mu.RUnlock()
		if action == nil {
			return nil, fmt.Errorf("action %q is not registered", step.Action)
		}
		return nil, action(ctx, step.ActionArguments)
	}
	if engine.sender == nil || !engine.sender.Ready() {
		return nil, fmt.Errorf("game websocket is not ready")
	}
	command := step.Command
	if command.Opcode == "" {
		command.Opcode = step.Opcode
	}
	if len(command.Payload) == 0 && len(step.Payload) > 0 {
		command.Payload = step.Payload
	}
	if command.Namespace == "" && !command.OmitNamespace {
		command.Namespace = engine.sender.Namespace()
	}
	concrete := step
	concrete.Opcode = command.Opcode
	concrete.Payload = command.Payload
	concrete.Command = command
	var err error
	afterRevision, _, err = engine.executeCommandDependencies(ctx, afterRevision, concrete)
	if err != nil {
		return nil, err
	}
	if !engine.sender.Ready() {
		return nil, fmt.Errorf("game websocket is not ready")
	}
	payload, err := Protocol.Encode(command)
	if err != nil {
		return nil, err
	}
	awaitOpcodes := stepAwaitOpcodes(step)
	responseTimeoutMillis := step.TimeoutMillis
	if responseTimeoutMillis <= 0 {
		responseTimeoutMillis = 10_000
	}
	responseToken := ""
	if provider, ok := engine.sender.(ResponseCorrelationProvider); ok && provider.CorrelatesResponses() && len(awaitOpcodes) > 0 {
		operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
		if operationID == "" {
			operationID = "response"
		}
		responseToken = fmt.Sprintf("%s/%d", operationID, engine.nextResponseID.Add(1))
	}
	var observed <-chan Protocol.CommittedFrame
	cancelWatch := func() {}
	if len(awaitOpcodes) > 0 {
		if engine.observer == nil {
			return nil, fmt.Errorf("response observer is unavailable")
		}
		if step.ResponseBarrier == ResponseBarrierWire || step.ResponseBarrier == ResponseBarrierWireThenCommitted {
			wireObserver, ok := engine.observer.(WireObserver)
			if !ok {
				return nil, fmt.Errorf("wire response observer is unavailable")
			}
			if responseToken != "" {
				if _, ok := engine.observer.(CorrelatedWireObserver); !ok {
					return nil, fmt.Errorf("correlated wire response observer is unavailable")
				}
			}
			observed, cancelWatch = engine.watchAnyWire(ctx, wireObserver, awaitOpcodes, responseToken)
		} else {
			if responseToken != "" {
				if _, ok := engine.observer.(CorrelatedObserver); !ok {
					return nil, fmt.Errorf("correlated response observer is unavailable")
				}
			}
			observed, cancelWatch = engine.watchAny(ctx, awaitOpcodes, afterRevision, responseToken)
		}
	}
	defer cancelWatch()
	sessionAtSend := engine.state.Session()
	connectionProvider, _ := engine.sender.(ConnectionGenerationProvider)
	connectionAtSend := uint64(0)
	if connectionProvider != nil {
		connectionAtSend = connectionProvider.ConnectionGeneration()
	}
	expectedConnection := Outbound.MetadataFromContext(ctx).ConnectionGeneration
	if expectedConnection == 0 {
		expectedConnection = sessionAtSend.ConnectionGeneration
	}
	if connectionAtSend > 0 && (expectedConnection == 0 || connectionAtSend != expectedConnection ||
		sessionAtSend.ConnectionGeneration != expectedConnection) {
		return nil, Outbound.ErrConnectionChanged
	}
	if sessionAtSend.Generation > 0 && !sessionHasAuthoritativeBaseline(sessionAtSend) {
		return nil, fmt.Errorf("game session baseline is not ready")
	}
	metadata := Outbound.MetadataFromContext(ctx)
	if expectedConnection > 0 || responseToken != "" {
		metadata.ConnectionGeneration = expectedConnection
		metadata.ResponseToken = responseToken
		metadata.ResponseOpcodes = append([]string(nil), awaitOpcodes...)
		metadata.ResponseTimeoutMillis = responseTimeoutMillis
	}
	sendContext := Outbound.WithMetadata(ctx, metadata)
	if err := validateDispatchPermit(ctx, afterRevision); err != nil {
		return nil, err
	}
	if err := advanceEffectPhase(ctx, EffectPhaseDispatching); err != nil {
		return nil, fmt.Errorf("persist dispatching effect: %w", err)
	}
	if err := engine.sender.Send(sendContext, payload); err != nil {
		if Outbound.IsIndeterminate(err) {
			_ = advanceEffectPhase(ctx, EffectPhaseReconciliationRequired)
		}
		return nil, err
	}
	var exchange *CommandExchange
	if step.CaptureResponse {
		exchange = &CommandExchange{Step: step.Name, Command: command}
	}
	if observed == nil {
		if err := advanceEffectPhase(ctx, EffectPhaseSent); err != nil {
			return exchange, Outbound.MarkIndeterminate(fmt.Errorf("persist sent effect: %w", err))
		}
		return exchange, nil
	}
	if err := advanceEffectPhase(ctx, EffectPhaseAwaitingResponse); err != nil {
		return exchange, Outbound.MarkIndeterminate(fmt.Errorf("persist response wait: %w", err))
	}
	var sessionChanges <-chan time.Time
	var sessionTicker *time.Ticker
	if expectedConnection > 0 || sessionAtSend.Generation > 0 {
		sessionTicker = time.NewTicker(sessionChangePollInterval)
		sessionChanges = sessionTicker.C
		defer sessionTicker.Stop()
	}
	timeout := time.Duration(responseTimeoutMillis) * time.Millisecond
	deadline := time.Now().Add(timeout)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	sessionChanged := func() bool {
		if expectedConnection > 0 && connectionProvider != nil {
			return connectionProvider.ConnectionGeneration() != expectedConnection || !engine.sender.Ready()
		}
		current := engine.state.Session()
		return current.Generation != sessionAtSend.Generation || !current.LoggedIn || !current.SocketReady
	}
	for {
		select {
		case <-ctx.Done():
			return exchange, Outbound.MarkIndeterminate(ctx.Err())
		case <-timer.C:
			return exchange, Outbound.MarkIndeterminate(fmt.Errorf("timed out waiting for %s", strings.Join(awaitOpcodes, " or ")))
		case <-sessionChanges:
			if sessionChanged() {
				return exchange, Outbound.MarkIndeterminate(fmt.Errorf("game session changed while waiting for %s", strings.Join(awaitOpcodes, " or ")))
			}
		case frame := <-observed:
			if step.ResponseBarrier == ResponseBarrierWire {
				if collector, _ := ctx.Value(wireCommitCollectorContextKey{}).(*wireCommitCollector); collector != nil {
					collector.add(frame.IngressID)
				} else if commitObserver, ok := engine.observer.(CommitObserver); ok {
					commitObserver.ForgetCommitted(frame.IngressID)
				}
			}
			var exactCommit CommitObserver
			if step.ResponseBarrier == ResponseBarrierWireThenCommitted {
				var ok bool
				exactCommit, ok = engine.observer.(CommitObserver)
				if !ok {
					return exchange, fmt.Errorf("committed wire response observer is unavailable")
				}
				defer exactCommit.ForgetCommitted(frame.IngressID)
			}
			if (expectedConnection > 0 || sessionAtSend.Generation > 0) && sessionChanged() {
				return exchange, Outbound.MarkIndeterminate(fmt.Errorf("game session changed while waiting for %s", strings.Join(awaitOpcodes, " or ")))
			}
			if len(step.SuccessCodes) > 0 {
				if frame.Frame.ResponseCode == nil || !containsInt(step.SuccessCodes, *frame.Frame.ResponseCode) {
					return exchange, fmt.Errorf("response code %v was not successful", frame.Frame.ResponseCode)
				}
			}
			if step.ResponseBarrier == ResponseBarrierWireThenCommitted {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return exchange, fmt.Errorf("timed out waiting for %s state commit", strings.Join(awaitOpcodes, " or "))
				}
				commitContext, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), remaining)
				committed, commitErr := exactCommit.WaitCommitted(commitContext, frame.IngressID)
				cancelCommit()
				if commitErr != nil {
					return exchange, fmt.Errorf("wait for %s state commit: %w", strings.Join(awaitOpcodes, " or "), commitErr)
				}
				frame = committed
				if (expectedConnection > 0 || sessionAtSend.Generation > 0) && sessionChanged() {
					return exchange, fmt.Errorf("game session changed while committing %s", strings.Join(awaitOpcodes, " or "))
				}
			}
			if frame.ReduceError != "" {
				return exchange, fmt.Errorf("response state reduction failed: %s", frame.ReduceError)
			}
			if exchange != nil {
				response := frame.Frame
				exchange.Response = &response
			}
			if err := advanceEffectPhase(ctx, EffectPhaseObserved); err != nil {
				return exchange, Outbound.MarkIndeterminate(fmt.Errorf("persist observed effect: %w", err))
			}
			return exchange, nil
		}
	}
}

func advanceEffectPhase(ctx context.Context, phase EffectPhase) error {
	callback, _ := ctx.Value(effectPhaseCallbackContextKey{}).(effectPhaseCallback)
	if callback == nil {
		return nil
	}
	return callback(phase)
}

func validateDispatchPermit(ctx context.Context, expectedRevision uint64) error {
	permit, _ := ctx.Value(dispatchPermitContextKey{}).(dispatchPermit)
	if permit == nil {
		return nil
	}
	return permit(expectedRevision)
}

func (engine *Engine) executeCommandDependencies(
	ctx context.Context,
	afterRevision uint64,
	step Step,
) (uint64, string, error) {
	opcode := strings.ToLower(strings.TrimSpace(step.Command.Opcode))
	engine.mu.RLock()
	resolver := engine.dependencies[opcode]
	engine.mu.RUnlock()
	if resolver == nil {
		return afterRevision, "", nil
	}
	planningInput := engine.planningContext()
	dependencies, err := resolver(ctx, planningInput, step)
	if err != nil {
		return afterRevision, "", err
	}
	dependencies.Key = strings.TrimSpace(dependencies.Key)
	if dependencies.Key == "" {
		return afterRevision, "", fmt.Errorf("%s command dependencies require a route key", opcode)
	}
	if receipt, _ := ctx.Value(commandDependencyReceiptContextKey{}).(commandDependencyReceipt); receipt.opcode != "" {
		if receipt.opcode != opcode || receipt.key != dependencies.Key {
			return afterRevision, "", fmt.Errorf("resolved %s command changed after its dependencies ran", opcode)
		}
		return engine.state.Revision(), dependencies.Key, nil
	}
	if opcode == "cra" {
		if provider, ok := engine.sender.(LaneAvailability); ok && provider != nil {
			if readyAt := provider.NextAllowed(Outbound.LaneAttackLaunch); readyAt.After(time.Now()) {
				timer := time.NewTimer(time.Until(readyAt))
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return afterRevision, "", ctx.Err()
				case <-timer.C:
				}
			}
		}
	}
	for _, dependency := range dependencies.Steps {
		currentRevision := engine.state.Revision()
		normalized := normalizePlan(Definition{}, currentRevision, Plan{Steps: []Step{dependency}})
		if len(normalized.Steps) != 1 {
			return afterRevision, "", fmt.Errorf("normalize %s command dependency", opcode)
		}
		if _, err := engine.executeStep(ctx, currentRevision, normalized.Steps[0]); err != nil {
			return afterRevision, "", fmt.Errorf("%s dependency %s: %w", opcode, stepLabel(normalized.Steps[0]), err)
		}
	}
	return engine.state.Revision(), dependencies.Key, nil
}

func (engine *Engine) fail(receipt Receipt, err error) Receipt {
	receipt.Status = StatusFailed
	receipt.Phase = EffectPhaseCompleted
	if Outbound.IsIndeterminate(err) && (receipt.Plan == nil || receipt.Plan.Effect != EffectRead) {
		receipt.Status = StatusIndeterminate
		receipt.Phase = EffectPhaseReconciliationRequired
	} else if errors.Is(err, context.Canceled) {
		receipt.Status = StatusCancelled
	}
	receipt.Error = err.Error()
	now := time.Now().UTC()
	receipt.CompletedAt = &now
	engine.update(receipt)
	return receipt
}

func (engine *Engine) failAfterProgress(receipt Receipt, err error, completedSteps map[string]int) Receipt {
	if Outbound.IsIndeterminate(err) || receipt.Plan == nil || receipt.Plan.Effect == EffectRead {
		return engine.fail(receipt, err)
	}
	for _, count := range completedSteps {
		if count > 0 {
			receipt.Status = StatusPartiallySucceeded
			receipt.Phase = EffectPhaseReconciliationRequired
			receipt.Error = err.Error()
			now := time.Now().UTC()
			receipt.CompletedAt = &now
			engine.update(receipt)
			return receipt
		}
	}
	return engine.fail(receipt, err)
}

func (engine *Engine) update(receipt Receipt) error {
	engine.mu.RLock()
	store := engine.operationStore
	engine.mu.RUnlock()
	var persistErr error
	if store != nil {
		persistErr = store.Save(context.Background(), receipt)
		engine.recordPersistenceResult(persistErr)
	}
	engine.mu.Lock()
	engine.publishLocked(receipt)
	engine.mu.Unlock()
	return persistErr
}

func (engine *Engine) publishLocked(receipt Receipt) {
	engine.cacheOperationLocked(receipt, "")
	engine.eventSequence++
	sequence := engine.eventSequence
	for _, subscriber := range engine.subscribers {
		delivery := receipt
		delivery.StreamSequence = sequence
		select {
		case subscriber <- delivery:
			continue
		default:
		}
		select {
		case <-subscriber:
			delivery.StreamGap = true
		default:
		}
		subscriber <- delivery
	}
}

func (engine *Engine) cacheOperationLocked(receipt Receipt, requestHash string) {
	id := strings.TrimSpace(receipt.ID)
	if id == "" {
		return
	}
	if _, indexed := engine.operationIndex[id]; !indexed {
		engine.operationIndex[id] = struct{}{}
		engine.operationOrder = append(engine.operationOrder, id)
	}
	engine.operations[id] = receipt
	if requestHash != "" {
		engine.requestHashes[id] = requestHash
	}
	engine.evictOperationHistoryLocked()
}

func (engine *Engine) evictOperationHistoryLocked() {
	if engine.operationStore == nil || len(engine.operations) <= operationHistoryLimit {
		return
	}
	remainingAttempts := len(engine.operationOrder)
	for len(engine.operations) > operationHistoryLimit && len(engine.operationOrder) > 0 && remainingAttempts > 0 {
		id := engine.operationOrder[0]
		engine.operationOrder = engine.operationOrder[1:]
		if _, active := engine.active[id]; active {
			engine.operationOrder = append(engine.operationOrder, id)
			remainingAttempts--
			continue
		}
		delete(engine.operations, id)
		delete(engine.requestHashes, id)
		delete(engine.operationIndex, id)
		remainingAttempts = len(engine.operationOrder)
	}
}

func (engine *Engine) publish(receipt Receipt) {
	engine.mu.Lock()
	engine.publishLocked(receipt)
	engine.mu.Unlock()
}

func normalizePlan(definition Definition, revision uint64, plan Plan) Plan {
	plan.Intent = definition.Name
	plan.Effect = definition.Effect
	plan.StateRevision = revision
	if plan.Admission != nil {
		admission := *plan.Admission
		admission.Class = AdmissionClass(strings.TrimSpace(string(admission.Class)))
		admission.Module = strings.TrimSpace(admission.Module)
		admission.Affinity = strings.TrimSpace(admission.Affinity)
		if admission.Class == "" {
			plan.Admission = nil
		} else {
			if admission.Module == "" {
				admission.Module = definition.Name
			}
			plan.Admission = &admission
		}
	}
	hasAttackLaunch := false
	for index := range plan.Steps {
		plan.Steps[index].Action = strings.TrimSpace(plan.Steps[index].Action)
		if plan.Steps[index].Action != "" && len(plan.Steps[index].ActionArguments) == 0 {
			plan.Steps[index].ActionArguments = json.RawMessage(`{}`)
		}
		plan.Steps[index].Resolver = strings.TrimSpace(plan.Steps[index].Resolver)
		if plan.Steps[index].Resolver != "" && len(plan.Steps[index].ResolverArguments) == 0 {
			plan.Steps[index].ResolverArguments = json.RawMessage(`{}`)
		}
		if plan.Steps[index].Opcode == "" {
			plan.Steps[index].Opcode = plan.Steps[index].Command.Opcode
		}
		if len(plan.Steps[index].Payload) == 0 && len(plan.Steps[index].Command.Payload) > 0 {
			plan.Steps[index].Payload = append(json.RawMessage(nil), plan.Steps[index].Command.Payload...)
		}
		plan.Steps[index].Opcode = strings.ToLower(plan.Steps[index].Opcode)
		plan.Steps[index].AwaitOpcode = strings.ToLower(plan.Steps[index].AwaitOpcode)
		plan.Steps[index].AwaitOpcodes = normalizeAwaitOpcodes(plan.Steps[index].AwaitOpcodes)
		plan.Steps[index].ResponseBarrier = ResponseBarrier(strings.ToLower(strings.TrimSpace(string(plan.Steps[index].ResponseBarrier))))
		if dependency := plan.Steps[index].CommandDependencies; dependency != nil {
			copy := *dependency
			copy.Opcode = strings.ToLower(strings.TrimSpace(copy.Opcode))
			if len(copy.Payload) == 0 {
				copy.Payload = json.RawMessage(`{}`)
			}
			plan.Steps[index].CommandDependencies = &copy
			if copy.Opcode == "cra" {
				hasAttackLaunch = true
			}
		}
		if plan.Steps[index].Opcode == "cra" || plan.Steps[index].Command.Opcode == "cra" {
			hasAttackLaunch = true
		}
		for _, opcode := range stepAwaitOpcodes(plan.Steps[index]) {
			plan.Claims = append(plan.Claims, "response:"+opcode)
			if opcode == "cra" {
				hasAttackLaunch = true
			}
		}
	}
	applyAutomaticResponseBarriers(plan.Steps)
	if hasAttackLaunch {
		plan.Claims = append(plan.Claims, "attack-context")
		if plan.Admission == nil {
			plan.Admission = &Admission{Class: AdmissionAttackLaunch, Module: definition.Name}
		}
	}
	plan.Claims = normalizeClaims(plan.Claims)
	return plan
}

func (engine *Engine) planningContext() PlanningContext {
	input := PlanningContext{}
	if provider, ok := engine.state.(interface{ PlanningView() State.PlanningView }); ok {
		view := provider.PlanningView()
		input.State = view.State
		input.Partitions = view.Partitions
		input.ProtocolContext = view.ProtocolContext
	} else if engine.state != nil {
		input.State = engine.state.Snapshot()
	}
	if engine.gameData != nil {
		input.GameData, _ = engine.gameData.Current()
	}
	return input
}

func finalizePlan(
	definition Definition,
	input PlanningContext,
	arguments json.RawMessage,
	plan Plan,
) (Plan, error) {
	plan = normalizePlan(definition, input.State.Revision, plan)
	plan.CatalogVersion = currentCatalogVersion(input)
	if definition.ReadSet != nil && input.Partitions.Available() {
		keys, err := definition.ReadSet(input, arguments, plan)
		if err != nil {
			return Plan{}, err
		}
		plan.Dependencies = append(plan.Dependencies, input.Dependencies(keys...)...)
	}
	plan.Dependencies = normalizePartitionDependencies(plan.Dependencies)
	plan.Resources = derivePlanResources(input.State, plan)
	if definition.RequireResources && len(plan.Steps) > 0 && !hasEffectResource(plan.Resources) {
		return Plan{}, fmt.Errorf("intent %q has executable steps but declares no effect resource", definition.Name)
	}
	if definition.RequireResources && hasLegacyResource(plan.Resources) {
		return Plan{}, fmt.Errorf("intent %q uses an unmapped legacy claim; declare a typed resource", definition.Name)
	}
	return plan, nil
}

func currentCatalogVersion(input PlanningContext) string {
	if input.GameData != nil {
		if version := strings.TrimSpace(input.GameData.Metadata().ItemVersion); version != "" {
			return version
		}
	}
	return strings.TrimSpace(input.State.CatalogVersion)
}

func hasEffectResource(resources []ResourceKey) bool {
	for _, resource := range resources {
		if resource.Scope == ResourceScopeSession && resource.Capability == "protocol" && resource.ResourceKind == "response" {
			continue
		}
		return true
	}
	return false
}

func normalizePartitionDependencies(dependencies []State.PartitionDependency) []State.PartitionDependency {
	latest := make(map[string]State.PartitionDependency, len(dependencies))
	for _, dependency := range dependencies {
		canonical := dependency.Key.Canonical()
		if dependency.Key.Capability == "" || dependency.Key.Scope.Kind == "" {
			continue
		}
		latest[canonical] = dependency
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]State.PartitionDependency, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}
	return out
}

func applyAutomaticResponseBarriers(steps []Step) {
	for start := 0; start < len(steps); {
		if !automaticWireCandidate(steps[start]) {
			start++
			continue
		}
		end := start + 1
		for end < len(steps) && automaticWireCandidate(steps[end]) {
			end++
		}
		if end-start > 1 {
			for index := start; index < end-1; index++ {
				steps[index].ResponseBarrier = ResponseBarrierWire
			}
			steps[end-1].ResponseBarrier = ResponseBarrierWireThenCommitted
		}
		start = end
	}
}

func automaticWireCandidate(step Step) bool {
	return step.ResponseBarrier == "" && step.Action == "" && step.Resolver == "" &&
		(step.Opcode != "" || step.Command.Opcode != "") && len(stepAwaitOpcodes(step)) > 0
}

func (engine *Engine) watchAny(
	ctx context.Context,
	opcodes []string,
	afterRevision uint64,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	watchContext, stop := context.WithCancel(ctx)
	merged := make(chan Protocol.CommittedFrame, 1)
	cancellations := make([]func(), 0, len(opcodes))
	for _, opcode := range opcodes {
		var source <-chan Protocol.CommittedFrame
		var cancel func()
		if responseToken != "" {
			source, cancel = engine.observer.(CorrelatedObserver).WatchResponse(opcode, afterRevision, responseToken)
		} else {
			source, cancel = engine.observer.Watch(opcode, afterRevision)
		}
		cancellations = append(cancellations, cancel)
		go func(frames <-chan Protocol.CommittedFrame) {
			select {
			case frame := <-frames:
				select {
				case merged <- frame:
				case <-watchContext.Done():
				}
			case <-watchContext.Done():
			}
		}(source)
	}
	var once sync.Once
	return merged, func() {
		once.Do(func() {
			stop()
			for _, cancel := range cancellations {
				cancel()
			}
		})
	}
}

func (engine *Engine) watchAnyWire(
	ctx context.Context,
	observer WireObserver,
	opcodes []string,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	watchContext, stop := context.WithCancel(ctx)
	merged := make(chan Protocol.CommittedFrame, 1)
	cancellations := make([]func(), 0, len(opcodes))
	commitObserver, canForget := engine.observer.(CommitObserver)
	var watchers sync.WaitGroup
	for _, opcode := range opcodes {
		var source <-chan Protocol.CommittedFrame
		var cancel func()
		if responseToken != "" {
			source, cancel = engine.observer.(CorrelatedWireObserver).WatchWireResponse(opcode, responseToken)
		} else {
			source, cancel = observer.WatchWire(opcode)
		}
		cancellations = append(cancellations, cancel)
		watchers.Add(1)
		go func(frames <-chan Protocol.CommittedFrame) {
			defer watchers.Done()
			select {
			case frame := <-frames:
				select {
				case merged <- frame:
				case <-watchContext.Done():
					if canForget {
						commitObserver.ForgetCommitted(frame.IngressID)
					}
				}
			case <-watchContext.Done():
			}
		}(source)
	}
	var once sync.Once
	return merged, func() {
		once.Do(func() {
			stop()
			for _, cancel := range cancellations {
				cancel()
			}
			watchers.Wait()
			if canForget {
				for {
					select {
					case frame := <-merged:
						commitObserver.ForgetCommitted(frame.IngressID)
					default:
						return
					}
				}
			}
		})
	}
}

func stepAwaitOpcodes(step Step) []string {
	opcodes := append([]string(nil), step.AwaitOpcodes...)
	if step.AwaitOpcode != "" {
		opcodes = append(opcodes, step.AwaitOpcode)
	}
	return normalizeAwaitOpcodes(opcodes)
}

func normalizeAwaitOpcodes(opcodes []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(opcodes))
	for _, opcode := range opcodes {
		opcode = strings.ToLower(strings.TrimSpace(opcode))
		if opcode == "" {
			continue
		}
		if _, exists := seen[opcode]; exists {
			continue
		}
		seen[opcode] = struct{}{}
		result = append(result, opcode)
	}
	return result
}

func stepLabel(step Step) string {
	if strings.TrimSpace(step.Name) != "" {
		return strings.TrimSpace(step.Name)
	}
	if step.Action != "" {
		return step.Action
	}
	if step.Resolver != "" {
		return step.Resolver
	}
	if step.Opcode != "" {
		return step.Opcode
	}
	return "intent step"
}

func sameStrings(left []string, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameAdmission(left, right *Admission) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sessionHasAuthoritativeBaseline(session State.SessionState) bool {
	return session.LoggedIn && session.SocketReady && session.Generation > 0 &&
		session.BaselineGeneration == session.Generation
}

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
