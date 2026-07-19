package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	allianceHelpHospitalType    = 2
	allianceHelpRecruitmentType = 6
	recruitmentProductionLineID = 0
	hospitalProductionLineID    = 2
)

type allianceHelpRequest struct {
	ProductionID int64          `json:"productionId"`
	CastleID     State.CastleID `json:"castleId,omitempty"`
	LineID       int            `json:"lineId,omitempty"`
}

func planAllianceHelpRequest(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	job, eligible := findAllianceHelpJob(input.State, request.ProductionID)
	if request.ProductionID <= 0 || !eligible {
		return Intent.Plan{}, fmt.Errorf("production job %d is not eligible for an alliance help request", request.ProductionID)
	}
	if job.LineID != recruitmentProductionLineID && job.LineID != hospitalProductionLineID {
		return Intent.Plan{}, fmt.Errorf("production line %d does not support alliance help requests", job.LineID)
	}
	castle, exists := input.State.Castles[job.CastleID]
	if !exists {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", job.CastleID)
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
	requestStep := commandStep("Request alliance help", "ahr", payload, "")
	requestStep.AwaitOpcodes = []string{"ahh", "ahr"}
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
	steps := castleContextSteps(castle)
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

func (application *Application) markAllianceHelpRequested(_ context.Context, arguments json.RawMessage) error {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.ProductionID <= 0 || application == nil || application.State == nil {
		return nil
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		if request.CastleID <= 0 {
			job, found := findAllianceHelpJob(*gameState, request.ProductionID)
			if !found {
				return nil, false, nil
			}
			request.CastleID = job.CastleID
			request.LineID = job.LineID
		}
		castle, exists := gameState.Castles[request.CastleID]
		if !exists {
			return nil, false, nil
		}
		queue, exists := castle.Production[request.LineID]
		if !exists {
			return nil, false, nil
		}
		changed := markAllianceHelpQueueRequested(&queue, request.LineID, request.ProductionID)
		castle.Production[request.LineID] = queue
		gameState.Castles[request.CastleID] = castle
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
				if allianceHelpLineSupported(lineID) && !queue.Active.AllianceHelpRequested {
					return job, true
				}
				foundJob = job
			}
			for _, item := range queue.Queued {
				if item.ProductionID == productionID {
					job := allianceHelpJob{CastleID: castleID, LineID: lineID}
					if allianceHelpLineSupported(lineID) && !item.AllianceHelpRequested {
						return job, true
					}
					foundJob = job
				}
			}
		}
	}
	return foundJob, false
}

func allianceHelpLineSupported(lineID int) bool {
	return lineID == recruitmentProductionLineID || lineID == hospitalProductionLineID
}

func allianceHelpEligible(state State.GameState, productionID int64) bool {
	job, eligible := findAllianceHelpJob(state, productionID)
	return eligible && allianceHelpLineSupported(job.LineID)
}
