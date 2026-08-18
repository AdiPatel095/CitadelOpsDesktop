package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
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
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists {
		if notice.TypeID != 3 {
			return Intent.Plan{}, fmt.Errorf("message %d is not a spy-report notice", request.MessageID)
		}
		if reportNoticeCannotBeFetched(notice) {
			return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable spy report %d", request.MessageID)}, nil
		}
	}
	payload, _ := json.Marshal(map[string]int64{"MID": request.MessageID})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Fetch spy report %d", request.MessageID),
		Steps:   []Intent.Step{commandStep("Fetch spy report", "bsd", payload, "bsd")},
	}, nil
}

func planSpyReportShare(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		MessageID int64 `json:"messageId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MessageID <= 0 {
		return Intent.Plan{}, fmt.Errorf("messageId is required")
	}
	if _, exists := input.State.LookupSpyReportCapture(request.MessageID); !exists {
		return Intent.Plan{}, fmt.Errorf("spy report %d is not available for sharing", request.MessageID)
	}
	recipients := make([]State.PlayerID, 0, len(input.State.Alliance.Members))
	seen := map[State.PlayerID]struct{}{}
	for _, member := range input.State.Alliance.Members {
		if member.PlayerID <= 0 || member.PlayerID == input.State.Player.ID {
			continue
		}
		if _, duplicate := seen[member.PlayerID]; duplicate {
			continue
		}
		seen[member.PlayerID] = struct{}{}
		recipients = append(recipients, member.PlayerID)
	}
	if len(recipients) == 0 {
		return Intent.Plan{}, fmt.Errorf("the current alliance has no report-share recipients")
	}
	payload, _ := json.Marshal(struct {
		MessageID  int64            `json:"MID"`
		Recipients []State.PlayerID `json:"PID"`
	}{MessageID: request.MessageID, Recipients: recipients})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Share spy report %d with %d alliance member(s)", request.MessageID, len(recipients)),
		Steps:   []Intent.Step{commandStep("Share spy report", "mfs", payload, "mfs")},
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
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists {
		if notice.TypeID != 6 {
			return Intent.Plan{}, fmt.Errorf("message %d is not a battle-report notice", request.MessageID)
		}
		if reportNoticeCannotBeFetched(notice) {
			return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable battle report %d", request.MessageID)}, nil
		}
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
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists && reportNoticeCannotBeFetched(notice) {
		return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable battle report %d", request.MessageID)}, nil
	}
	capture, exists := input.State.LookupBattleReportCapture(request.MessageID)
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

func reportNoticeCannotBeFetched(notice State.ReportNotice) bool {
	switch notice.Status {
	case "archived", "expired", "ignored", "unavailable":
		return true
	default:
		return false
	}
}
