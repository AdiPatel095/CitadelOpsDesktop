package Intent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
)

type Engine struct {
	registry *Registry
	state    StateReader
	gameData GameDataProvider
	sender   Sender
	observer Observer
	claims   *claimManager

	mu          sync.RWMutex
	actions     map[string]Action
	operations  map[string]Receipt
	subscribers map[uint64]chan Receipt
	nextID      atomic.Uint64
	nextSubID   atomic.Uint64
}

func NewEngine(registry *Registry, state StateReader, gameData GameDataProvider, sender Sender, observer Observer) *Engine {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Engine{
		registry: registry, state: state, gameData: gameData, sender: sender, observer: observer,
		claims: newClaimManager(), actions: map[string]Action{}, operations: map[string]Receipt{}, subscribers: map[uint64]chan Receipt{},
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

func (engine *Engine) Submit(ctx context.Context, request Request) Receipt {
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
	receipt := Receipt{
		ID: request.ID, Intent: request.Name, Actor: request.Actor,
		Status: StatusPlanning, SubmittedAt: time.Now().UTC(),
	}
	engine.update(receipt)

	definition, exists := engine.registry.Definition(request.Name)
	if !exists {
		return engine.fail(receipt, fmt.Errorf("unknown intent %q", request.Name))
	}
	if engine.state == nil {
		return engine.fail(receipt, fmt.Errorf("state store is unavailable"))
	}
	initial := engine.state.Snapshot()
	if request.ExpectedRevision != nil && initial.Revision != *request.ExpectedRevision {
		return engine.fail(receipt, fmt.Errorf("state revision changed: expected %d, current %d", *request.ExpectedRevision, initial.Revision))
	}
	var gameDataStore *GameData.Store
	if engine.gameData != nil {
		gameDataStore, _ = engine.gameData.Current()
	}
	plan, err := definition.Planner(ctx, PlanningContext{State: initial, GameData: gameDataStore}, request.Arguments)
	if err != nil {
		return engine.fail(receipt, err)
	}
	plan = normalizePlan(definition, initial.Revision, plan)
	receipt.Plan = &plan
	if request.DryRun {
		receipt.Status = StatusPlanned
		now := time.Now().UTC()
		receipt.CompletedAt = &now
		engine.update(receipt)
		return receipt
	}

	release, err := engine.claims.acquire(ctx, plan.Claims)
	if err != nil {
		return engine.fail(receipt, err)
	}
	defer release()

	current := engine.state.Snapshot()
	if request.ExpectedRevision != nil && current.Revision != *request.ExpectedRevision {
		return engine.fail(receipt, fmt.Errorf("state revision changed while waiting for claims: expected %d, current %d", *request.ExpectedRevision, current.Revision))
	}
	if engine.gameData != nil {
		gameDataStore, _ = engine.gameData.Current()
	}
	revalidated, err := definition.Planner(ctx, PlanningContext{State: current, GameData: gameDataStore}, request.Arguments)
	if err != nil {
		return engine.fail(receipt, err)
	}
	revalidated = normalizePlan(definition, current.Revision, revalidated)
	if !sameStrings(plan.Claims, revalidated.Claims) {
		return engine.fail(receipt, fmt.Errorf("intent claims changed during revalidation"))
	}
	plan = revalidated
	receipt.Plan = &plan
	started := time.Now().UTC()
	receipt.StartedAt = &started
	receipt.Status = StatusRunning
	engine.update(receipt)

	for _, step := range plan.Steps {
		if err := engine.executeStep(ctx, current.Revision, step); err != nil {
			return engine.fail(receipt, fmt.Errorf("%s: %w", stepLabel(step), err))
		}
		current = engine.state.Snapshot()
	}
	receipt.Status = StatusSucceeded
	completed := time.Now().UTC()
	receipt.CompletedAt = &completed
	engine.update(receipt)
	return receipt
}

func (engine *Engine) Operation(id string) (Receipt, bool) {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	receipt, ok := engine.operations[id]
	return receipt, ok
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

func (engine *Engine) executeStep(ctx context.Context, afterRevision uint64, step Step) error {
	if step.Action != "" {
		engine.mu.RLock()
		action := engine.actions[step.Action]
		engine.mu.RUnlock()
		if action == nil {
			return fmt.Errorf("action %q is not registered", step.Action)
		}
		return action(ctx, step.ActionArguments)
	}
	if engine.sender == nil || !engine.sender.Ready() {
		return fmt.Errorf("game websocket is not ready")
	}
	command := step.Command
	if command.Opcode == "" {
		command.Opcode = step.Opcode
	}
	if len(command.Payload) == 0 && len(step.Payload) > 0 {
		command.Payload = step.Payload
	}
	if command.Namespace == "" {
		command.Namespace = engine.sender.Namespace()
	}
	payload, err := Protocol.Encode(command)
	if err != nil {
		return err
	}
	var observed <-chan Protocol.CommittedFrame
	cancelWatch := func() {}
	if step.AwaitOpcode != "" {
		if engine.observer == nil {
			return fmt.Errorf("response observer is unavailable")
		}
		observed, cancelWatch = engine.observer.Watch(step.AwaitOpcode, afterRevision)
	}
	defer cancelWatch()
	if err := engine.sender.Send(ctx, payload); err != nil {
		return err
	}
	if observed == nil {
		return nil
	}
	timeout := time.Duration(step.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for %s", step.AwaitOpcode)
	case frame := <-observed:
		if len(step.SuccessCodes) > 0 {
			if frame.Frame.ResponseCode == nil || !containsInt(step.SuccessCodes, *frame.Frame.ResponseCode) {
				return fmt.Errorf("response code %v was not successful", frame.Frame.ResponseCode)
			}
		}
		return nil
	}
}

func (engine *Engine) fail(receipt Receipt, err error) Receipt {
	receipt.Status = StatusFailed
	receipt.Error = err.Error()
	now := time.Now().UTC()
	receipt.CompletedAt = &now
	engine.update(receipt)
	return receipt
}

func (engine *Engine) update(receipt Receipt) {
	engine.mu.Lock()
	engine.operations[receipt.ID] = receipt
	for _, subscriber := range engine.subscribers {
		select {
		case subscriber <- receipt:
		default:
		}
	}
	engine.mu.Unlock()
}

func normalizePlan(definition Definition, revision uint64, plan Plan) Plan {
	plan.Intent = definition.Name
	plan.Effect = definition.Effect
	plan.StateRevision = revision
	plan.Claims = normalizeClaims(plan.Claims)
	for index := range plan.Steps {
		plan.Steps[index].Action = strings.TrimSpace(plan.Steps[index].Action)
		if plan.Steps[index].Action != "" && len(plan.Steps[index].ActionArguments) == 0 {
			plan.Steps[index].ActionArguments = json.RawMessage(`{}`)
		}
		if plan.Steps[index].Opcode == "" {
			plan.Steps[index].Opcode = plan.Steps[index].Command.Opcode
		}
		if len(plan.Steps[index].Payload) == 0 && len(plan.Steps[index].Command.Payload) > 0 {
			plan.Steps[index].Payload = append(json.RawMessage(nil), plan.Steps[index].Command.Payload...)
		}
		plan.Steps[index].Opcode = strings.ToLower(plan.Steps[index].Opcode)
		plan.Steps[index].AwaitOpcode = strings.ToLower(plan.Steps[index].AwaitOpcode)
	}
	return plan
}

func stepLabel(step Step) string {
	if strings.TrimSpace(step.Name) != "" {
		return strings.TrimSpace(step.Name)
	}
	if step.Action != "" {
		return step.Action
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

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
