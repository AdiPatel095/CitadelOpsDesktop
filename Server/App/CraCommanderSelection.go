package App

import (
	"fmt"
	"sort"
	"strings"

	"CitadelDesktop/Server/State"
)

const maximumCRACommanderCount = 50

type craCommanderSelectionRequest struct {
	Candidates []State.CommanderID `json:"candidates,omitempty"`
	Count      int                 `json:"count,omitempty"`
	Strategy   string              `json:"strategy,omitempty"`
}

type craCommanderSelectionOptions struct {
	DefaultCandidates []State.CommanderID
	DefaultCount      int
	Eligible          map[State.CommanderID]struct{}
	RequireAvailable  bool
}

type craCommanderResolution struct {
	Selected   []State.CommanderID
	Candidates []State.CommanderID
	Strategy   string
}

func resolveCRACommanders(
	gameState State.GameState,
	selection *craCommanderSelectionRequest,
	options craCommanderSelectionOptions,
) (craCommanderResolution, error) {
	strategy := "first_available"
	if selection != nil && strings.TrimSpace(selection.Strategy) != "" {
		strategy = strings.ToLower(strings.TrimSpace(selection.Strategy))
	}
	if strategy != "first_available" && strategy != "lowest_id" && strategy != "highest_id" {
		return craCommanderResolution{}, fmt.Errorf("unknown CRA commander selection strategy %q", strategy)
	}

	var requested []State.CommanderID
	switch {
	case selection != nil && len(selection.Candidates) > 0:
		requested = selection.Candidates
	case selection != nil:
		requested = allCommanderIDs(gameState)
	case options.DefaultCandidates != nil:
		requested = options.DefaultCandidates
	default:
		requested = allCommanderIDs(gameState)
	}
	candidates, err := validatedCommanderCandidates(gameState, requested)
	if err != nil {
		return craCommanderResolution{}, err
	}
	if len(candidates) == 0 {
		return craCommanderResolution{}, fmt.Errorf("CRA commander selection has no candidates")
	}

	ordered := append([]State.CommanderID(nil), candidates...)
	switch strategy {
	case "lowest_id":
		sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	case "highest_id":
		sort.Slice(ordered, func(left, right int) bool { return ordered[left] > ordered[right] })
	}

	count := options.DefaultCount
	if selection != nil && selection.Count != 0 {
		count = selection.Count
	}
	if count <= 0 {
		return craCommanderResolution{}, fmt.Errorf("CRA commander selection count must be positive")
	}
	if count > maximumCRACommanderCount {
		return craCommanderResolution{}, fmt.Errorf("CRA commander selection count may not exceed %d", maximumCRACommanderCount)
	}

	selected := make([]State.CommanderID, 0, count)
	for _, id := range ordered {
		if options.Eligible != nil {
			if _, eligible := options.Eligible[id]; !eligible {
				continue
			}
		}
		if options.RequireAvailable && !gameState.Commanders[id].Available {
			continue
		}
		selected = append(selected, id)
		if len(selected) == count {
			break
		}
	}
	if len(selected) != count {
		availability := ""
		if options.RequireAvailable {
			availability = " available"
		}
		return craCommanderResolution{}, fmt.Errorf(
			"CRA commander selection requested %d but only %d%s candidate(s) matched",
			count, len(selected), availability,
		)
	}
	return craCommanderResolution{Selected: selected, Candidates: candidates, Strategy: strategy}, nil
}

func validatedCommanderCandidates(gameState State.GameState, requested []State.CommanderID) ([]State.CommanderID, error) {
	seen := map[State.CommanderID]struct{}{}
	result := make([]State.CommanderID, 0, len(requested))
	for _, id := range requested {
		if id < 0 {
			return nil, fmt.Errorf("CRA commander candidate %d is invalid", id)
		}
		if _, exists := gameState.Commanders[id]; !exists {
			return nil, fmt.Errorf("commander %d is not in the current player state", id)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func allCommanderIDs(gameState State.GameState) []State.CommanderID {
	result := make([]State.CommanderID, 0, len(gameState.Commanders))
	for id := range gameState.Commanders {
		if id >= 0 {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}
