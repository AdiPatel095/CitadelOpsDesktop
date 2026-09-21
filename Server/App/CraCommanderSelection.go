package App

import (
	"CitadelDesktop/Server/Localization"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const maximumCRACommanderCount = 50

var errCRACommanderUnavailable = Localization.WithError(errors.New("CRA commander availability changed"), Localization.New("server.app.cra_commander_availability_changed.267a988b", "CRA commander availability changed", nil))

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
	// Holds skips commanders that a just-planned launch already took (their
	// movement is not yet visible in state) and registers the selected ones
	// for craCommanderLaunchHold, so launch bursts never re-select a
	// commander into a CRA 256.
	Holds Intent.CommanderHoldRegistry
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
		return craCommanderResolution{}, Localization.WithError(fmt.Errorf("unknown CRA commander selection strategy %q", strategy), Localization.New("server.app.unknown_cra_commander_selection.81c6ce74", "unknown CRA commander selection strategy {p0}", Localization.Params{"p0": fmt.Sprintf("%q", strategy)}))
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
		return craCommanderResolution{}, Localization.WithError(fmt.Errorf("CRA commander selection has no candidates"), Localization.New("server.app.cra_commander_selection_has.0ef36902", "CRA commander selection has no candidates", nil))
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
		return craCommanderResolution{}, Localization.WithError(fmt.Errorf("CRA commander selection count must be positive"), Localization.New("server.app.cra_commander_selection_count.d9a3cb12", "CRA commander selection count must be positive", nil))
	}
	if count > maximumCRACommanderCount {
		return craCommanderResolution{}, Localization.WithError(fmt.Errorf("CRA commander selection count may not exceed %d", maximumCRACommanderCount), Localization.New("server.app.cra_commander_selection_count.56cc2b94", "CRA commander selection count may not exceed {p0}", Localization.Params{"p0": maximumCRACommanderCount}))
	}

	now := time.Now().UTC()
	selected := make([]State.CommanderID, 0, count)
	for _, id := range ordered {
		if options.Eligible != nil {
			if _, eligible := options.Eligible[id]; !eligible {
				continue
			}
		}
		if options.RequireAvailable && (!gameState.Commanders[id].Available ||
			State.CommanderHasActiveMovementAt(gameState, id, now) ||
			State.InvasionCommanderReserved(gameState, id)) {
			continue
		}
		if options.Holds != nil && options.Holds.CommanderHeldAt(id, now) {
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
		err := Localization.WithError(fmt.Errorf(
			"CRA commander selection requested %d but only %d%s candidate(s) matched",
			count, len(selected), availability,
		), Localization.New("server.app.cra_commander_selection_requested.a38166f6", "CRA commander selection requested {p0} but only {p1}{p2} candidate(s) matched", Localization.Params{"p0": count, "p1": len(selected), "p2": fmt.Sprintf("%s", availability)}))
		if options.RequireAvailable {
			return craCommanderResolution{}, fmt.Errorf("%w: %v", errCRACommanderUnavailable, err)
		}
		return craCommanderResolution{}, err
	}
	// NOTE: holds are consulted but deliberately NOT registered here. Plan
	// validation re-runs this selection for the same operation before
	// dispatch; registering at selection time made the re-plan see its own
	// commander as taken and fail permanently stale on single-commander
	// configurations. Until registration moves to the dispatch layer, the
	// CRA 256 combat cooldown is the sole (and sufficient) burst defense.
	return craCommanderResolution{Selected: selected, Candidates: candidates, Strategy: strategy}, nil
}

func validatedCommanderCandidates(gameState State.GameState, requested []State.CommanderID) ([]State.CommanderID, error) {
	seen := map[State.CommanderID]struct{}{}
	result := make([]State.CommanderID, 0, len(requested))
	for _, id := range requested {
		if id < 0 {
			return nil, Localization.WithError(fmt.Errorf("CRA commander candidate %d is invalid", id), Localization.New("server.app.cra_commander_candidate_p.9d686fbf", "CRA commander candidate {p0} is invalid", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		if _, exists := gameState.Commanders[id]; !exists {
			return nil, Localization.WithError(fmt.Errorf("commander %d is not in the current player state", id), Localization.New("server.app.commander_p_is_not.62a21327", "commander {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
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
