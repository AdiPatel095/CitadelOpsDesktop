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
	mu       sync.RWMutex
	reducers map[string]Reducer
}

func NewRegistry() *Registry {
	return &Registry{reducers: map[string]Reducer{}}
}

func (registry *Registry) Register(opcode string, reducer Reducer) error {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || reducer == nil {
		return fmt.Errorf("opcode and reducer are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.reducers[opcode]; exists {
		return fmt.Errorf("reducer already registered for %s", opcode)
	}
	registry.reducers[opcode] = reducer
	return nil
}

func (registry *Registry) reducer(opcode string) Reducer {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.reducers[strings.ToLower(opcode)]
}
