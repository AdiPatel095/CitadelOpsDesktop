package App

import (
	"CitadelDesktop/Server/Localization"
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("messageId is required"), Localization.New("server.app.messageid_is_required.44a498df", "messageId is required", nil))
	}
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists {
		if notice.TypeID != 3 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("message %d is not a spy-report notice", request.MessageID), Localization.New("server.app.message_p_is_not.c17cc727", "message {p0} is not a spy-report notice", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}))
		}
		if reportNoticeCannotBeFetched(notice) {
			return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable spy report %d", request.MessageID), SummaryDescriptor: Localization.New("server.app.skip_unavailable_spy_report.e61f58d3", "Skip unavailable spy report {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)})}, nil
		}
	}
	payload, _ := json.Marshal(map[string]int64{"MID": request.MessageID})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Fetch spy report %d", request.MessageID), SummaryDescriptor: Localization.New("server.app.fetch_spy_report_p.415e31fa", "Fetch spy report {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}),
		Steps: []Intent.Step{commandStep("Fetch spy report", "bsd", payload, "bsd", Localization.New("server.app.fetch_spy_report.0a635f4b", "Fetch spy report", nil))},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("messageId is required"), Localization.New("server.app.messageid_is_required.44a498df", "messageId is required", nil))
	}
	if _, exists := input.State.LookupSpyReportCapture(request.MessageID); !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("spy report %d is not available for sharing", request.MessageID), Localization.New("server.app.spy_report_p_is.82933140", "spy report {p0} is not available for sharing", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}))
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the current alliance has no report-share recipients"), Localization.New("server.app.the_current_alliance_has.fc95c0f4", "the current alliance has no report-share recipients", nil))
	}
	payload, _ := json.Marshal(struct {
		MessageID  int64            `json:"MID"`
		Recipients []State.PlayerID `json:"PID"`
	}{MessageID: request.MessageID, Recipients: recipients})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Share spy report %d with %d alliance member(s)", request.MessageID, len(recipients)), SummaryDescriptor: Localization.New("server.app.share_spy_report_p.cf8549c7", "Share spy report {p0} with {p1} alliance member(s)", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID), "p1": len(recipients)}),
		Steps: []Intent.Step{commandStep("Share spy report", "mfs", payload, "mfs", Localization.New("server.app.share_spy_report.311f6387", "Share spy report", nil))},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("messageId is required"), Localization.New("server.app.messageid_is_required.44a498df", "messageId is required", nil))
	}
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists {
		if notice.TypeID != 6 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("message %d is not a battle-report notice", request.MessageID), Localization.New("server.app.message_p_is_not.09fb3bbe", "message {p0} is not a battle-report notice", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}))
		}
		if reportNoticeCannotBeFetched(notice) {
			return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable battle report %d", request.MessageID), SummaryDescriptor: Localization.New("server.app.skip_unavailable_battle_report.7dbf5edc", "Skip unavailable battle report {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)})}, nil
		}
	}
	payload, _ := json.Marshal(struct {
		MessageID int64 `json:"MID"`
		InboxMode int   `json:"IM"`
	}{request.MessageID, 0})
	return Intent.Plan{
		Claims:  []string{"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10)},
		Summary: fmt.Sprintf("Fetch battle report summary %d", request.MessageID), SummaryDescriptor: Localization.New("server.app.fetch_battle_report_summary.9c56446c", "Fetch battle report summary {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}),
		Steps: []Intent.Step{commandStep("Fetch battle report summary", "bls", payload, "bls", Localization.New("server.app.fetch_battle_report_summary.d9898df9", "Fetch battle report summary", nil))},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("messageId and reportId are required"), Localization.New("server.app.messageid_and_reportid_are.ef2550d6", "messageId and reportId are required", nil))
	}
	if notice, exists := input.State.LookupReportNotice(request.MessageID); exists && reportNoticeCannotBeFetched(notice) {
		return Intent.Plan{Summary: fmt.Sprintf("Skip unavailable battle report %d", request.MessageID), SummaryDescriptor: Localization.New("server.app.skip_unavailable_battle_report.7dbf5edc", "Skip unavailable battle report {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)})}, nil
	}
	capture, exists := input.State.LookupBattleReportCapture(request.MessageID)
	if !exists || capture.ReportID != request.ReportID || len(capture.Summary) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("battle report %d summary context is unavailable", request.MessageID), Localization.New("server.app.battle_report_p_summary.a70d4729", "battle report {p0} summary context is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.MessageID)}))
	}
	payload, _ := json.Marshal(map[string]int64{"LID": request.ReportID})
	return Intent.Plan{
		Claims: []string{
			"reports", "report-message:" + strconv.FormatInt(request.MessageID, 10),
			"battle-report:" + strconv.FormatInt(request.ReportID, 10),
		},
		Summary: fmt.Sprintf("Fetch battle report details %d", request.ReportID), SummaryDescriptor: Localization.New("server.app.fetch_battle_report_details.8affe96f", "Fetch battle report details {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.ReportID)}),
		Steps: []Intent.Step{
			commandStep("Fetch battle report waves", "blm", payload, "blm", Localization.New("server.app.fetch_battle_report_waves.14c86765", "Fetch battle report waves", nil)),
			commandStep("Fetch battle report units and tools", "bld", payload, "bld", Localization.New("server.app.fetch_battle_report_units.9792f393", "Fetch battle report units and tools", nil)),
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
