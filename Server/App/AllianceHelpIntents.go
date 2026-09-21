package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

const (
	allianceHelpHospitalType    = 2
	allianceHelpRecruitmentType = 6
	recruitmentProductionLineID = 0
	hospitalProductionLineID    = 2
	allianceHelpAllLimit        = 15
)

type allianceHelpRequest struct {
	ProductionID int64          `json:"productionId"`
	CastleID     State.CastleID `json:"castleId,omitempty"`
	LineID       int            `json:"lineId,omitempty"`
}

type recruitmentBUPAllianceHelpRequest struct {
	CastleID       State.CastleID `json:"castleId"`
	RequireCovered bool           `json:"requireCovered,omitempty"`
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
			return Intent.Plan{Summary: "Skip alliance help: no alliance member currently needs help", SummaryDescriptor: Localization.New("server.app.skip_alliance_help_no.eb86e43c", "Skip alliance help: no alliance member currently needs help", nil)}, nil
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
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.help_p_pending_alliance.2dc560fc", "Help {p0} pending alliance request(s)", Localization.Params{"p0": fmt.Sprintf("%d", len(listIDs))})
	if len(listIDs) == 0 {
		summary = "Check and help current alliance requests"
		summaryLocalizationMessage = Localization.New("server.app.check_and_help_current.f4cb37d0", "Check and help current alliance requests", nil)
	}
	return Intent.Plan{
		Claims:  []string{"alliance-help"},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: []Intent.Step{
			{
				Name: "Help alliance members", NameDescriptor: Localization.New("server.app.help_alliance_members.2b3dae8c", "Help alliance members", nil), Resolver: "alliance.help.answer_all.build",
				ResolverArguments: recordArguments, AwaitOpcode: "aha", TimeoutMillis: 10_000,
				SuccessCodes: []int{0},
			},
			{
				Name: "Record answered alliance help", NameDescriptor: Localization.New("server.app.record_answered_alliance_help.5c94942b", "Record answered alliance help", nil), Action: "alliance.help.mark_answered",
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: alliance-help session changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4717979", "intent plan became stale before dispatch: alliance-help session changed", nil))
	}
	pending := State.PendingOtherAllianceHelpListIDs(input.State)
	currentObservation := input.State.AllianceHelpRequests.OthersObservedGeneration == input.State.Session.Generation &&
		!input.State.AllianceHelpRequests.OthersObservedAt.IsZero()
	bootstrapAllowed := request.AllowUnobserved &&
		input.State.AllianceHelpRequests.LastHelpAllGeneration != input.State.Session.Generation &&
		(!currentObservation || len(pending) > 0)
	if !allianceHelpListsOverlap(request.ListIDs, pending) && !bootstrapAllowed {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: the selected alliance-help requests are no longer pending", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.e3b606ea", "intent plan became stale before dispatch: the selected alliance-help requests are no longer pending", nil))
	}
	payload, _ := json.Marshal(struct {
		Limit int `json:"KID"`
	}{Limit: allianceHelpAllLimit})
	return commandStep("Help alliance members", "aha", payload, "aha", Localization.New("server.app.help_alliance_members.2b3dae8c", "Help alliance members", nil)), nil
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("alliance help requires a positive production job id"), Localization.New("server.app.alliance_help_requires_a.eec16f9d", "alliance help requires a positive production job id", nil))
	}
	job, eligible := findAllianceHelpJob(input.State, request.ProductionID)
	if !eligible {
		if job.CastleID > 0 && !allianceHelpLineSupported(job.LineID) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("production line %d does not support alliance help requests", job.LineID), Localization.New("server.app.production_line_p_does.cc1eadf7", "production line {p0} does not support alliance help requests", Localization.Params{"p0": fmt.Sprintf("%d", job.LineID)}))
		}
		return Intent.Plan{Summary: fmt.Sprintf(
			"Skip alliance help: production job %d is no longer eligible", request.ProductionID,
		), SummaryDescriptor: Localization.New("server.app.skip_alliance_help_production.ef4e2b38", "Skip alliance help: production job {p0} is no longer eligible", Localization.Params{"p0": fmt.Sprintf("%d", request.ProductionID)})}, nil
	}
	if job.LineID != recruitmentProductionLineID && job.LineID != hospitalProductionLineID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production line %d does not support alliance help requests", job.LineID), Localization.New("server.app.production_line_p_does.cc1eadf7", "production line {p0} does not support alliance help requests", Localization.Params{"p0": fmt.Sprintf("%d", job.LineID)}))
	}
	if job.LineID == hospitalProductionLineID &&
		State.OutstandingHospitalAllianceHelpRequests(input.State) >=
			State.MaximumOutstandingHospitalAllianceHelpRequests {
		return Intent.Plan{Summary: "Skip alliance help: hospital already has an outstanding request", SummaryDescriptor: Localization.New("server.app.skip_alliance_help_hospital.d12487ed", "Skip alliance help: hospital already has an outstanding request", nil)}, nil
	}
	castle, exists := input.State.Castles[job.CastleID]
	if !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", job.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", job.CastleID)}))
	}
	if State.CastleFocusKnownUnavailable(input.State, castle) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: castle %d cannot be focused in the current kingdom session", Intent.ErrPlanStale, job.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.f50ee7dc", "intent plan became stale before dispatch: castle {p1} cannot be focused in the current kingdom session", Localization.Params{"p1": fmt.Sprintf("%d", job.CastleID)}))
	}
	if job.LineID == hospitalProductionLineID && !State.OwnAllianceHelpListCurrent(input.State) {
		return Intent.Plan{Summary: "Skip alliance help: waiting for the current hospital request list", SummaryDescriptor: Localization.New("server.app.skip_alliance_help_waiting.7bd5e4c3", "Skip alliance help: waiting for the current hospital request list", nil)}, nil
	}
	if job.LineID == recruitmentProductionLineID &&
		!recruitmentAllianceHelpQueueCurrent(input.State, castle, time.Now().UTC()) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d needs a current production queue",
			Intent.ErrPlanStale, job.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.1c26c16c", "intent plan became stale before dispatch: recruitment castle {p1} needs a current production queue", Localization.Params{"p1": fmt.Sprintf("%d", job.CastleID)}))
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
		Name: "Request alliance help", NameDescriptor: Localization.New("server.app.request_alliance_help.bb51b355", "Request alliance help", nil), Resolver: "alliance.help.build", ResolverArguments: recordArguments,
		AwaitOpcodes: []string{"ahh", "ahr"}, TimeoutMillis: 10_000, SuccessCodes: []int{0},
	}
	focusStep := castleFocusStep(castle)
	focusStep.StaleCodes = []int{175}
	steps := []Intent.Step{focusStep}
	if job.LineID == hospitalProductionLineID {
		steps = append(steps, requestStep, Intent.Step{
			Name: "Record alliance help request", NameDescriptor: Localization.New("server.app.record_alliance_help_request.4797e958", "Record alliance help request", nil), Action: "alliance.help.mark_requested", ActionArguments: recordArguments,
		})
	} else {
		steps = append(steps, requestStep)
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: "Request alliance help for the active production queue", SummaryDescriptor: Localization.New("server.app.request_alliance_help_for.8df98bd1", "Request alliance help for the active production queue", nil),
		Steps: steps,
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("alliance help requires a positive production job id"), Localization.New("server.app.alliance_help_requires_a.eec16f9d", "alliance help requires a positive production job id", nil))
	}
	job, eligible := findAllianceHelpJob(input.State, request.ProductionID)
	if !eligible || job.CastleID != request.CastleID || job.LineID != request.LineID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: production job %d is no longer eligible for alliance help", Intent.ErrPlanStale, request.ProductionID,
		), Localization.New("server.app.intent_plan_became_stale.77e5e464", "intent plan became stale before dispatch: production job {p1} is no longer eligible for alliance help", Localization.Params{"p1": fmt.Sprintf("%d", request.ProductionID)}))
	}
	castle, exists := input.State.Castles[job.CastleID]
	if !exists || State.CastleFocusKnownUnavailable(input.State, castle) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: castle %d cannot be focused in the current kingdom session", Intent.ErrPlanStale, job.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.f50ee7dc", "intent plan became stale before dispatch: castle {p1} cannot be focused in the current kingdom session", Localization.Params{"p1": fmt.Sprintf("%d", job.CastleID)}))
	}
	if job.LineID == recruitmentProductionLineID &&
		!recruitmentAllianceHelpQueueCurrent(input.State, castle, time.Now().UTC()) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d needs a current production queue",
			Intent.ErrPlanStale, job.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.1c26c16c", "intent plan became stale before dispatch: recruitment castle {p1} needs a current production queue", Localization.Params{"p1": fmt.Sprintf("%d", job.CastleID)}))
	}
	if job.LineID == recruitmentProductionLineID && !recruitmentAllianceHelpContextCurrent(input, castle) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d is not the current committed castle context",
			Intent.ErrPlanStale, job.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.abdda625", "intent plan became stale before dispatch: recruitment castle {p1} is not the current committed castle context", Localization.Params{"p1": fmt.Sprintf("%d", job.CastleID)}))
	}
	if job.LineID == hospitalProductionLineID && !State.OwnAllianceHelpListCurrent(input.State) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: hospital alliance help needs the current request list", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.b6ede151", "intent plan became stale before dispatch: hospital alliance help needs the current request list", nil))
	}
	if job.LineID == hospitalProductionLineID &&
		State.OutstandingHospitalAllianceHelpRequests(input.State) >=
			State.MaximumOutstandingHospitalAllianceHelpRequests {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: hospital alliance help already has an outstanding request",
			Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.157e8c13", "intent plan became stale before dispatch: hospital alliance help already has an outstanding request", nil))
	}
	if job.LineID == recruitmentProductionLineID {
		return recruitmentAllianceHelpCommand(input, request.CastleID), nil
	}
	payload, _ := json.Marshal(struct {
		RequestID int64 `json:"ID"`
		Type      int   `json:"T"`
	}{RequestID: request.ProductionID, Type: allianceHelpHospitalType})
	step := commandStep("Request alliance help", "ahr", payload, "", Localization.New("server.app.request_alliance_help.bb51b355", "Request alliance help", nil))
	step.AwaitOpcodes = []string{"ahh", "ahr"}
	step.StaleCodes = []int{175}
	return step, nil
}

func (application *Application) resolveRecruitmentBUPAllianceHelpStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request recruitmentBUPAllianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || State.CastleFocusKnownUnavailable(input.State, castle) ||
		!recruitmentAllianceHelpContextCurrent(input, castle) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d is not the current committed castle context",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.abdda625", "intent plan became stale before dispatch: recruitment castle {p1} is not the current committed castle context", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	// The BUP response itself is the causal evidence for this request. Do not
	// suppress it with focus-epoch markers or an older alliance-help lifecycle;
	// the game may safely reject a duplicate. The current player/castle context
	// above and the transport response identity remain mandatory.
	return recruitmentAllianceHelpCommand(input, request.CastleID), nil
}

func recruitmentAllianceHelpCommand(input Intent.PlanningContext, castleID State.CastleID) Intent.Step {
	payload, _ := json.Marshal(struct {
		RequestID int64 `json:"ID"`
		Type      int   `json:"T"`
	}{RequestID: 0, Type: allianceHelpRecruitmentType})
	step := commandStep("Request alliance help", "ahr", payload, "", Localization.New("server.app.request_alliance_help.bb51b355", "Request alliance help", nil))
	step.AwaitOpcodes = []string{"ahh", "ahr"}
	step.StaleCodes = []int{175}
	step.ResponseIdentity = Outbound.ResponseIdentity{
		PlayerID: int64(input.State.Player.ID), CastleID: int64(castleID),
	}
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func (application *Application) markRecruitmentBUPAllianceHelpDue(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request recruitmentBUPAllianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("production state is unavailable"), Localization.New("server.app.production_state_is_unavailable.46c36af4", "production state is unavailable", nil))
	}
	view := application.State.PlanningView()
	castle, exists := view.State.Castles[request.CastleID]
	input := Intent.PlanningContext{State: view.State, ProtocolContext: view.ProtocolContext}
	if !exists || !recruitmentAllianceHelpContextCurrent(input, castle) {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d focus changed after BUP",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.e8f87db0", "intent plan became stale before dispatch: recruitment castle {p1} focus changed after BUP", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	protocol := view.ProtocolContext
	if !application.State.ObserveRecruitmentBUP(
		request.CastleID, protocol.SessionGeneration, protocol.ConnectionGeneration, protocol.FocusEpoch,
	) {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment castle %d focus changed while recording BUP",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.11af907a", "intent plan became stale before dispatch: recruitment castle {p1} focus changed while recording BUP", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	if request.RequireCovered {
		protocol = application.State.ProtocolContext()
		if protocol.RecruitmentBUPCastleID != request.CastleID ||
			protocol.RecruitmentBUPFocusEpoch != protocol.FocusEpoch ||
			protocol.RecruitmentBUPSerial == 0 ||
			protocol.RecruitmentAHRCoveredSerial != protocol.RecruitmentBUPSerial {
			return Localization.WithError(fmt.Errorf(
				"%w: recruitment alliance-help coverage expired after BUP at castle %d",
				Intent.ErrPlanStale, request.CastleID,
			), Localization.New("server.app.intent_plan_became_stale.a3b0fe21", "intent plan became stale before dispatch: recruitment alliance-help coverage expired after BUP at castle {p1}", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
		}
	}
	return nil
}

func (application *Application) markRecruitmentBUPAllianceHelpCovered(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request recruitmentBUPAllianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("production state is unavailable"), Localization.New("server.app.production_state_is_unavailable.46c36af4", "production state is unavailable", nil))
	}
	view := application.State.PlanningView()
	castle, exists := view.State.Castles[request.CastleID]
	input := Intent.PlanningContext{State: view.State, ProtocolContext: view.ProtocolContext}
	protocol := view.ProtocolContext
	if !exists || !recruitmentAllianceHelpContextCurrent(input, castle) ||
		!State.RecruitmentAllianceHelpCovers(view.State, request.CastleID, time.Now().UTC(), 0) ||
		protocol.RecruitmentBUPCastleID != request.CastleID ||
		protocol.RecruitmentBUPFocusEpoch != protocol.FocusEpoch ||
		protocol.RecruitmentBUPSerial == 0 ||
		protocol.RecruitmentBUPSerial <= protocol.RecruitmentAHRCoveredSerial {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment BUP batch at castle %d changed before AHR confirmation",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.1eab9dcf", "intent plan became stale before dispatch: recruitment BUP batch at castle {p1} changed before AHR confirmation", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	if !application.State.ObserveRecruitmentAHRCovered(
		request.CastleID,
		protocol.SessionGeneration,
		protocol.ConnectionGeneration,
		protocol.FocusEpoch,
		protocol.RecruitmentBUPSerial,
	) {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment BUP batch at castle %d changed while confirming AHR",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.b56b3b26", "intent plan became stale before dispatch: recruitment BUP batch at castle {p1} changed while confirming AHR", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	return nil
}

func (application *Application) reconcileStandaloneRecruitmentBUPAllianceHelp(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.CastleID <= 0 || request.LineID != recruitmentProductionLineID {
		return nil
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("production state is unavailable"), Localization.New("server.app.production_state_is_unavailable.46c36af4", "production state is unavailable", nil))
	}
	view := application.State.PlanningView()
	castle, exists := view.State.Castles[request.CastleID]
	input := Intent.PlanningContext{State: view.State, ProtocolContext: view.ProtocolContext}
	protocol := view.ProtocolContext
	if !exists || !recruitmentAllianceHelpContextCurrent(input, castle) ||
		!application.State.ObserveStandaloneRecruitmentAHRCovered(
			request.CastleID,
			protocol.SessionGeneration,
			protocol.ConnectionGeneration,
			protocol.FocusEpoch,
			time.Now().UTC(),
		) {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment alliance-help lifecycle was not committed in castle %d focus",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.587b0e08", "intent plan became stale before dispatch: recruitment alliance-help lifecycle was not committed in castle {p1} focus", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	return nil
}

func (application *Application) prepareStandaloneRecruitmentBUPAllianceHelp(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.CastleID <= 0 || request.LineID != recruitmentProductionLineID {
		return nil
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("production state is unavailable"), Localization.New("server.app.production_state_is_unavailable.46c36af4", "production state is unavailable", nil))
	}
	view := application.State.PlanningView()
	castle, exists := view.State.Castles[request.CastleID]
	input := Intent.PlanningContext{State: view.State, ProtocolContext: view.ProtocolContext}
	protocol := view.ProtocolContext
	if !exists || !recruitmentAllianceHelpContextCurrent(input, castle) ||
		!application.State.PrepareStandaloneRecruitmentAHR(
			request.CastleID,
			protocol.SessionGeneration,
			protocol.ConnectionGeneration,
			protocol.FocusEpoch,
		) {
		return Localization.WithError(fmt.Errorf(
			"%w: recruitment AHR could not bind castle %d focus",
			Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.1b8a42bc", "intent plan became stale before dispatch: recruitment AHR could not bind castle {p1} focus", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	return nil
}

func recruitmentAllianceHelpQueueCurrent(state State.GameState, castle State.CastleState, now time.Time) bool {
	queue, exists := castle.Production[recruitmentProductionLineID]
	return exists && !State.ProductionQueueNeedsRefresh(state, queue, now) &&
		!State.ProductionQueuePredatesCastleSnapshot(castle, queue)
}

func recruitmentAllianceHelpContextCurrent(input Intent.PlanningContext, castle State.CastleState) bool {
	protocol := input.ProtocolContext
	session := input.State.Session
	return input.State.Player.ID > 0 && castle.ID > 0 && castle.Focused &&
		session.Generation > 0 && session.ConnectionGeneration > 0 &&
		protocol.SessionGeneration == session.Generation &&
		protocol.ConnectionGeneration == session.ConnectionGeneration &&
		protocol.FocusEpoch > 0 &&
		protocol.FocusedCastleID == castle.ID && protocol.FocusSubcontext == State.FocusSubcontextCastle
}

func (application *Application) markAllianceHelpRequested(_ context.Context, arguments json.RawMessage) error {
	var request allianceHelpRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.ProductionID <= 0 || application == nil || application.State == nil {
		return nil
	}
	_, err := application.State.ApplyComponents(
		State.Components(State.ComponentCastles, State.ComponentAllianceHelp),
		func(gameState *State.GameState) ([]string, bool, error) {
			if request.CastleID <= 0 {
				job, found := findAllianceHelpJob(*gameState, request.ProductionID)
				if !found {
					return nil, false, nil
				}
				request.CastleID = job.CastleID
				request.LineID = job.LineID
			}
			// Recruitment help is recorded only by the game's own matching ahh
			// event. A completed command step is not sufficient evidence because
			// unrelated alliance broadcasts can share that opcode.
			if request.LineID == recruitmentProductionLineID {
				return nil, false, nil
			}
			if request.LineID != hospitalProductionLineID {
				return nil, false, nil
			}
			stateChanged := State.ReconcileOwnHospitalAllianceHelp(gameState, request.ProductionID, true)
			castle, exists := gameState.MutableCastleParts(request.CastleID, State.CastlePartProduction)
			if !exists {
				return []string{"alliance-help"}, stateChanged, nil
			}
			queue, exists := castle.Production[request.LineID]
			if !exists {
				return []string{"alliance-help"}, stateChanged, nil
			}
			queueChanged := markAllianceHelpQueueRequested(&queue, request.LineID, request.ProductionID)
			castle.Production[request.LineID] = queue
			gameState.SetCastleParts(request.CastleID, castle, State.CastlePartProduction)
			return []string{"alliance-help", "castles", "production"}, stateChanged || queueChanged, nil
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
		return true
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
