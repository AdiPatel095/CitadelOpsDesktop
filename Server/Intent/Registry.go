package Intent

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{definitions: map[string]Definition{}}
}

func (registry *Registry) Register(definition Definition) error {
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Name == "" || definition.Planner == nil {
		return fmt.Errorf("intent name and planner are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.definitions[definition.Name]; exists {
		return fmt.Errorf("intent %q is already registered", definition.Name)
	}
	registry.definitions[definition.Name] = definition
	return nil
}

func (registry *Registry) Definition(name string) (Definition, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	definition, ok := registry.definitions[name]
	return definition, ok
}

func (registry *Registry) Definitions() []Definition {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	definitions := make([]Definition, 0, len(registry.definitions))
	for _, definition := range registry.definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].Name < definitions[right].Name
	})
	return definitions
}
