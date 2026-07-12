package Ingest

import (
	"context"
	"fmt"
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

type Registry struct {
	mu               sync.RWMutex
	inboundReducers  map[string]Reducer
	outboundReducers map[string]Reducer
}

func NewRegistry() *Registry {
	return &Registry{
		inboundReducers:  map[string]Reducer{},
		outboundReducers: map[string]Reducer{},
	}
}

func (registry *Registry) Register(opcode string, reducer Reducer) error {
	return registry.register(opcode, reducer, registry.inboundReducers)
}

func (registry *Registry) RegisterOutbound(opcode string, reducer Reducer) error {
	return registry.register(opcode, reducer, registry.outboundReducers)
}

func (registry *Registry) register(opcode string, reducer Reducer, reducers map[string]Reducer) error {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || reducer == nil {
		return fmt.Errorf("opcode and reducer are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := reducers[opcode]; exists {
		return fmt.Errorf("reducer already registered for %s", opcode)
	}
	reducers[opcode] = reducer
	return nil
}

func (registry *Registry) reducer(opcode string, direction Protocol.Direction) Reducer {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if direction == Protocol.DirectionOutbound {
		return registry.outboundReducers[strings.ToLower(opcode)]
	}
	return registry.inboundReducers[strings.ToLower(opcode)]
}
