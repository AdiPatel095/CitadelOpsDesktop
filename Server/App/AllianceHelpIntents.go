package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	allianceHelpHospitalType    = 2
	allianceHelpRecruitmentType = 6
	recruitmentProductionLineID = 0
	hospitalProductionLineID    = 2
	hospitalAllianceHelpLimit   = 3
	allianceHelpAllLimit        = 15
)

type allianceHelpRequest struct {
	ProductionID int64          `json:"productionId"`
	CastleID     State.CastleID `json:"castleId,omitempty"`
	LineID       int            `json:"lineId,omitempty"`
}

type allianceHelpAnswerAllRequest struct {
	ListIDs           []int64 `json:"listIds"`
	SessionGeneration uint64  `json:"sessionGeneration"`
	AllowUnobserved   bool    `json:"allowUnobserved,omitempty"`
}

func planAllianceHelpAnswerAll(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var options struct {
		AllowUnobserved bool `json:"allowUnobserved,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &options); err != nil {
		return Intent.Plan{}, err
	}
	listIDs := State.PendingOtherAllianceHelpListIDs(input.State)
	if len(listIDs) == 0 {
		if !options.AllowUnobserved || input.State.Session.Generation == 0 ||
			input.State.AllianceHelpRequests.LastHelpAllGeneration == input.State.Session.Generation {
			return Intent.Plan{Summary: "Skip alliance help: no alliance member currently needs help"}, nil
		}
		listIDs = []int64{}
	}
	if len(listIDs) > allianceHelpAllLimit {
		listIDs = listIDs[:allianceHelpAllLimit]
	}
	request := allianceHelpAnswerAllRequest{
		ListIDs: listIDs, SessionGeneration: input.State.Session.Generation, AllowUnobserved: options.AllowUnobserved,
	}
	recordArguments, _ := json.Marshal(request)
	summary := fmt.Sprintf("Help %d pending alliance request(s)", len(listIDs))
	if len(listIDs) == 0 {
		summary = "Check and help current alliance requests"
	}
	return Intent.Plan{
		Claims:  []string{"alliance-help"},
		Summary: summary,
		Steps: []Intent.Step{
			{
				Name: "Help alliance members", Resolver: "alliance.help.answer_all.build",
				ResolverArguments: recordArguments, AwaitOpcode: "aha", TimeoutMillis: 10_000,
				SuccessCodes: []int{0},
			},
			{
				Name: "Record answered alliance help", Action: "alliance.help.mark_answered",
				ActionArguments: recordArguments,
			},
		},
	}, nil
}

func resolveAllianceHelpAnswerAllStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request allianceHelpAnswerAllRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if request.SessionGeneration == 0 || request.SessionGeneration != input.State.Session.Generation {
		return Intent.Step{}, fmt.Errorf("%w: alliance-help session changed", Intent.ErrPlanStale)
	}
	pending := State.PendingOtherAllianceHelpListIDs(input.State)
	currentObservation := input.State.AllianceHelpRequests.OthersObservedGeneration == input.State.Session.Generation &&
		!input.State.AllianceHelpRequests.OthersObservedAt.IsZero()
	bootstrapAllowed := request.AllowUnobserved &&
		input.State.AllianceHelpRequests.LastHelpAllGeneration != input.State.Session.Generation &&
		(!currentObservation || len(pending) > 0)
	if !allianceHelpListsOverlap(request.ListIDs, pending) && !bootstrapAllowed {
		return Intent.Step{}, fmt.Errorf("%w: the selected alliance-help requests are no longer pending", Intent.ErrPlanStale)
	}
	payload, _ := json.Marshal(struct {
		Limit int `json:"KID"`
	}{Limit: allianceHelpAllLimit})
	return commandStep("Help alliance members", "aha", payload, "aha"), nil
}

func (application *Application) markAllianceHelpAnswered(_ context.Context, arguments json.RawMessage) error {
	var request allianceHelpAnswerAllRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil || request.SessionGeneration == 0 {
		return nil
	}
	answered := make(map[int64]struct{}, len(request.ListIDs))
	for _, listID := range request.ListIDs {
		if listID > 0 {
			answered[listID] = struct{}{}
		}
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentAllianceHelp), func(gameState *State.GameState) ([]string, bool, error) {
		if gameState.Session.Generation != request.SessionGeneration {
			return nil, false, nil
		}
		gameState.AllianceHelpRequests.LastHelpAllGeneration = request.SessionGeneration
		gameState.AllianceHelpRequests.LastHelpAllAt = time.Now().UTC()
		changed := true
		current := gameState.AllianceHelpRequests.PendingOtherListIDs
		remaining := make([]int64, 0, len(current))
		for _, listID := range current {
			if gameState.AllianceHelpRequests.OthersObservedGeneration != request.SessionGeneration {
				remaining = append(remaining, listID)
				continue
			}
			if _, found := answered[listID]; !found {
				remaining = append(remaining, listID)
			}
		}
		if len(remaining) != len(current) {
			gameState.AllianceHelpRequests.PendingOtherListIDs = remaining
			changed = true
		}
		return []string{"alliance-help"}, changed, nil
	})
	return err
}

func allianceHelpListsOverlap(left []int64, right []int64) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	available := make(map[int64]struct{}, len(right))
	for _, listID := range right {
		if listID > 0 {
			available[listID] = struct{}{}
		}
	}
	for _, listID := range left {
		if _, found := available[listID]; found {
			return true
		}
	}
	return false
}

func planAllianceHelpRequest(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.ProductionID <= 0 {
		return Intent.Plan{}, fmt.Errorf("alliance help requires a positive production job id")
	}
	job, eligible := findAllianceHelpJob(input.State, request.ProductionID)
	if !eligible {
		if job.CastleID > 0 && !allianceHelpLineSupported(job.LineID) {
			return Intent.Plan{}, fmt.Errorf("production line %d does not support alliance help requests", job.LineID)
		}
		return Intent.Plan{Summary: fmt.Sprintf(
			"Skip alliance help: production job %d is no longer eligible", request.ProductionID,
		)}, nil
	}
	if job.LineID != recruitmentProductionLineID && job.LineID != hospitalProductionLineID {
		return Intent.Plan{}, fmt.Errorf("production line %d does not support alliance help requests", job.LineID)
	}
	if job.LineID == hospitalProductionLineID &&
		State.OutstandingHospitalAllianceHelpRequests(input.State) >= hospitalAllianceHelpLimit {
		return Intent.Plan{Summary: fmt.Sprintf(
			"Skip alliance help: hospital already has %d outstanding requests", hospitalAllianceHelpLimit,
		)}, nil
	}
	castle, exists := input.State.Castles[job.CastleID]
	if !exists {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", job.CastleID)
	}
	recordRequest := allianceHelpRequest{
		ProductionID: request.ProductionID,
		CastleID:     job.CastleID,
		LineID:       job.LineID,
	}
	recordArguments, _ := json.Marshal(recordRequest)
	claims := []string{
		"alliance-help", "castle-focus",
		"alliance-help:" + strconv.FormatInt(request.ProductionID, 10),
		"castle:" + strconv.FormatInt(int64(job.CastleID), 10),
	}
	if job.LineID == hospitalProductionLineID {
		claims = append(claims, "hospital")
	} else {
		claims = append(claims, "production-line:"+strconv.Itoa(job.LineID))
	}
	requestStep := Intent.Step{
		Name: "Request alliance help", Resolver: "alliance.help.build", ResolverArguments: recordArguments,
		AwaitOpcodes: []string{"ahh", "ahr"}, TimeoutMillis: 10_000, SuccessCodes: []int{0},
	}
	steps := []Intent.Step{castleFocusStep(castle)}
	steps = append(steps,
		requestStep,
		Intent.Step{Name: "Record alliance help request", Action: "alliance.help.mark_requested", ActionArguments: recordArguments},
	)
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Request alliance help for production job %d", request.ProductionID),
		Steps:   steps,
	}, nil
}

func (application *Application) resolveAllianceHelpRequestStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if request.ProductionID <= 0 {
		return Intent.Step{}, fmt.Errorf("alliance help requires a positive production job id")
	}
	job, eligible := findAllianceHelpJob(input.State, request.ProductionID)
	if !eligible || job.CastleID != request.CastleID || job.LineID != request.LineID {
		return Intent.Step{}, fmt.Errorf(
			"%w: production job %d is no longer eligible for alliance help", Intent.ErrPlanStale, request.ProductionID,
		)
	}
	if job.LineID == hospitalProductionLineID &&
		State.OutstandingHospitalAllianceHelpRequests(input.State) >= hospitalAllianceHelpLimit {
		return Intent.Step{}, fmt.Errorf(
			"%w: hospital alliance help already has %d outstanding requests",
			Intent.ErrPlanStale, hospitalAllianceHelpLimit,
		)
	}
	requestID := request.ProductionID
	requestType := allianceHelpHospitalType
	if job.LineID == recruitmentProductionLineID {
		requestID = 0
		requestType = allianceHelpRecruitmentType
	}
	payload, _ := json.Marshal(struct {
		RequestID int64 `json:"ID"`
		Type      int   `json:"T"`
	}{RequestID: requestID, Type: requestType})
	step := commandStep("Request alliance help", "ahr", payload, "")
	step.AwaitOpcodes = []string{"ahh", "ahr"}
	return step, nil
}

func (application *Application) markAllianceHelpRequested(_ context.Context, arguments json.RawMessage) error {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.ProductionID <= 0 || application == nil || application.State == nil {
		return nil
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentCastles), func(gameState *State.GameState) ([]string, bool, error) {
		if request.CastleID <= 0 {
			job, found := findAllianceHelpJob(*gameState, request.ProductionID)
			if !found {
				return nil, false, nil
			}
			request.CastleID = job.CastleID
			request.LineID = job.LineID
		}
		castle, exists := gameState.MutableCastleParts(request.CastleID, State.CastlePartProduction)
		if !exists {
			return nil, false, nil
		}
		queue, exists := castle.Production[request.LineID]
		if !exists {
			return nil, false, nil
		}
		changed := markAllianceHelpQueueRequested(&queue, request.LineID, request.ProductionID)
		castle.Production[request.LineID] = queue
		gameState.SetCastleParts(request.CastleID, castle, State.CastlePartProduction)
		return []string{"castles", "production"}, changed, nil
	})
	return err
}

func markAllianceHelpQueueRequested(queue *State.ProductionQueue, lineID int, productionID int64) bool {
	if queue == nil {
		return false
	}
	markAll := lineID == recruitmentProductionLineID
	changed := false
	if queue.Active != nil && (markAll || queue.Active.ProductionID == productionID) && !queue.Active.AllianceHelpRequested {
		queue.Active.AllianceHelpRequested = true
		changed = true
	}
	for index := range queue.Queued {
		if (markAll || queue.Queued[index].ProductionID == productionID) && !queue.Queued[index].AllianceHelpRequested {
			queue.Queued[index].AllianceHelpRequested = true
			changed = true
		}
	}
	return changed
}

type allianceHelpJob struct {
	CastleID State.CastleID
	LineID   int
}

func findAllianceHelpJob(state State.GameState, productionID int64) (allianceHelpJob, bool) {
	foundJob := allianceHelpJob{}
	for castleID, castle := range state.Castles {
		for lineID, queue := range castle.Production {
			if queue.Active != nil && queue.Active.ProductionID == productionID {
				job := allianceHelpJob{CastleID: castleID, LineID: lineID}
				if allianceHelpJobEligible(state, castleID, lineID, *queue.Active) {
					return job, true
				}
				foundJob = job
			}
			for _, item := range queue.Queued {
				if item.ProductionID == productionID {
					job := allianceHelpJob{CastleID: castleID, LineID: lineID}
					if allianceHelpJobEligible(state, castleID, lineID, item) {
						return job, true
					}
					foundJob = job
				}
			}
		}
	}
	return foundJob, false
}

func allianceHelpJobEligible(
	state State.GameState,
	castleID State.CastleID,
	lineID int,
	item State.QueueItem,
) bool {
	if !allianceHelpLineSupported(lineID) || item.AllianceHelpRequested {
		return false
	}
	if lineID == recruitmentProductionLineID {
		return !State.HasOutstandingRecruitmentAllianceHelpRequest(state, castleID)
	}
	return lineID != hospitalProductionLineID ||
		!State.HasOutstandingHospitalAllianceHelpRequest(state, item.ProductionID)
}

func allianceHelpLineSupported(lineID int) bool {
	return lineID == recruitmentProductionLineID || lineID == hospitalProductionLineID
}

func allianceHelpEligible(state State.GameState, productionID int64) bool {
	job, eligible := findAllianceHelpJob(state, productionID)
	return eligible && allianceHelpLineSupported(job.LineID)
}
