package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
)

func planSpyReportFetch(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		MessageID int64 `json:"messageId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MessageID <= 0 {
		return Intent.Plan{}, fmt.Errorf("messageId is required")
	}
	if notice, exists := input.State.Reports.Notices[request.MessageID]; exists && notice.TypeID != 3 {
		return Intent.Plan{}, fmt.Errorf("message %d is not a spy-report notice", request.MessageID)
	}
	payload, _ := json.Marshal(map[string]int64{"MID": request.MessageID})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Fetch spy report %d", request.MessageID),
		Steps:   []Intent.Step{commandStep("Fetch spy report", "bsd", payload, "bsd")},
	}, nil
}

func planBattleReportSummary(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		MessageID int64 `json:"messageId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MessageID <= 0 {
		return Intent.Plan{}, fmt.Errorf("messageId is required")
	}
	if notice, exists := input.State.Reports.Notices[request.MessageID]; exists && notice.TypeID != 6 {
		return Intent.Plan{}, fmt.Errorf("message %d is not a battle-report notice", request.MessageID)
	}
	payload, _ := json.Marshal(struct {
		MessageID int64 `json:"MID"`
		InboxMode int   `json:"IM"`
	}{request.MessageID, 0})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Fetch battle report summary %d", request.MessageID),
		Steps:   []Intent.Step{commandStep("Fetch battle report summary", "bls", payload, "bls")},
	}, nil
}

func planBattleReportDetails(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		MessageID int64 `json:"messageId"`
		ReportID  int64 `json:"reportId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MessageID <= 0 || request.ReportID <= 0 {
		return Intent.Plan{}, fmt.Errorf("messageId and reportId are required")
	}
	capture, exists := input.State.Reports.BattleCaptures[request.MessageID]
	if !exists || capture.ReportID != request.ReportID || len(capture.Summary) == 0 {
		return Intent.Plan{}, fmt.Errorf("battle report %d summary context is unavailable", request.MessageID)
	}
	payload, _ := json.Marshal(map[string]int64{"LID": request.ReportID})
	return Intent.Plan{
		Claims: []string{
			"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10),
			"battle-report:" + strconv.FormatInt(request.ReportID, 10),
		},
		Summary: fmt.Sprintf("Fetch battle report details %d", request.ReportID),
		Steps: []Intent.Step{
			commandStep("Fetch battle report waves", "blm", payload, "blm"),
			commandStep("Fetch battle report units and tools", "bld", payload, "bld"),
		},
	}, nil
}
