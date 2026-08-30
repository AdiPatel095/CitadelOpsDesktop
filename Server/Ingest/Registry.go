package Ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type Reducer func(
	ctx context.Context,
	frame Protocol.Frame,
	state *State.GameState,
	gameData *GameData.Store,
) (domains []string, changed bool, err error)

type registeredReducer struct {
	reducer Reducer
	writes  State.ComponentSet
	steps   []reducerStep
}

type reducerStep struct {
	reducer Reducer
	writes  State.ComponentSet
}

type Registry struct {
	mu               sync.RWMutex
	inboundReducers  map[string]registeredReducer
	outboundReducers map[string]registeredReducer
}

func (registry *Registry) HasInbound(opcode string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, exists := registry.inboundReducers[strings.ToLower(strings.TrimSpace(opcode))]
	return exists
}

func (registry *Registry) HasOutbound(opcode string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, exists := registry.outboundReducers[strings.ToLower(strings.TrimSpace(opcode))]
	return exists
}

func (registry *Registry) InboundOpcodes() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	opcodes := make([]string, 0, len(registry.inboundReducers))
	for opcode := range registry.inboundReducers {
		opcodes = append(opcodes, opcode)
	}
	sort.Strings(opcodes)
	return opcodes
}

func NewRegistry() *Registry {
	return &Registry{
		inboundReducers:  map[string]registeredReducer{},
		outboundReducers: map[string]registeredReducer{},
	}
}

func (registry *Registry) Register(opcode string, reducer Reducer) error {
	return registry.RegisterComponents(opcode, State.AllComponents, reducer)
}

func (registry *Registry) RegisterOutbound(opcode string, reducer Reducer) error {
	return registry.RegisterOutboundComponents(opcode, State.AllComponents, reducer)
}

func (registry *Registry) RegisterComponents(opcode string, writes State.ComponentSet, reducer Reducer) error {
	return registry.register(opcode, writes, reducer, registry.inboundReducers)
}

func (registry *Registry) RegisterOutboundComponents(opcode string, writes State.ComponentSet, reducer Reducer) error {
	return registry.register(opcode, writes, reducer, registry.outboundReducers)
}

func (registry *Registry) register(
	opcode string,
	writes State.ComponentSet,
	reducer Reducer,
	reducers map[string]registeredReducer,
) error {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || reducer == nil {
		return fmt.Errorf("opcode and reducer are required")
	}
	if writes == 0 || writes&^State.AllComponents != 0 {
		return fmt.Errorf("valid reducer write components are required for %s", opcode)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := reducers[opcode]; exists {
		return fmt.Errorf("reducer already registered for %s", opcode)
	}
	reducers[opcode] = registeredReducer{
		reducer: reducer, writes: writes,
		steps: []reducerStep{{reducer: reducer, writes: writes}},
	}
	return nil
}

// registerComponentSequence retains per-reducer write ownership for opcodes
// whose official response contains multiple logical projections. The Store
// still clones the union before reduction, but persistence and client deltas
// include only the steps that actually changed state.
func (registry *Registry) registerComponentSequence(opcode string, steps ...reducerStep) error {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || len(steps) == 0 {
		return fmt.Errorf("opcode and reducer steps are required")
	}
	writes := State.ComponentSet(0)
	reducers := make([]Reducer, 0, len(steps))
	owned := make([]reducerStep, 0, len(steps))
	for _, step := range steps {
		if step.reducer == nil || step.writes == 0 || step.writes&^State.AllComponents != 0 {
			return fmt.Errorf("valid reducer step write components are required for %s", opcode)
		}
		writes = writes.Union(step.writes)
		reducers = append(reducers, step.reducer)
		owned = append(owned, step)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.inboundReducers[opcode]; exists {
		return fmt.Errorf("reducer already registered for %s", opcode)
	}
	registry.inboundReducers[opcode] = registeredReducer{
		reducer: combineReducers(reducers...), writes: writes, steps: owned,
	}
	return nil
}

func (registry *Registry) reducer(opcode string, direction Protocol.Direction) Reducer {
	return registry.registered(opcode, direction).reducer
}

func (registry *Registry) registered(opcode string, direction Protocol.Direction) registeredReducer {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if direction == Protocol.DirectionOutbound {
		return registry.outboundReducers[strings.ToLower(opcode)]
	}
	return registry.inboundReducers[strings.ToLower(opcode)]
}
