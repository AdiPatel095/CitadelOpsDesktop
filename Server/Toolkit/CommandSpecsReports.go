package Toolkit

import (
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/GameCommands"
)

type battleSummaryArgs struct {
	MessageID int64 `json:"messageId"`
	InboxMode int   `json:"inboxMode"`
}

type reportIDArgs struct {
	ReportID int64 `json:"reportId"`
}

type reportMessageIDArgs struct {
	MessageID int64 `json:"messageId"`
}

type spyInfoArgs struct {
	TargetX   int `json:"targetX"`
	TargetY   int `json:"targetY"`
	KingdomID int `json:"kingdomId"`
}

type shareReportArgs struct {
	MessageID int64 `json:"messageId"`
	PlayerIDs []int `json:"playerIds"`
}

type customEmptyArgs struct {
	Opcode string `json:"opcode"`
}

func registerReportCommands(builder *commandSpecBuilder) error {
	if err := builder.add("bls", "bls", "Request a battle-report summary by inbox message ID.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"messageId": schemaProperty("integer", "Battle report inbox MID."),
			"inboxMode": schemaProperty("integer", "Wire IM value; shared reports commonly use 0."),
		}, "messageId", "inboxMode"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[battleSummaryArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive64(args.MessageID, "messageId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.BLSPayload(args.MessageID, args.InboxMode)), nil
		}); err != nil {
		return err
	}

	reportSchema := objectSchema(map[string]interface{}{"reportId": schemaProperty("integer", "Report LID returned by BLS.")}, "reportId")
	if err := builder.add("blm", "blm", "Request per-wave battle aggregates by report LID.", EffectGameQuery, reportSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[reportIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive64(args.ReportID, "reportId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.BLMPayload(args.ReportID)), nil
		}); err != nil {
		return err
	}
	if err := builder.add("bld", "bld", "Request detailed battle-report unit and tool rows by report LID.", EffectGameQuery, reportSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[reportIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive64(args.ReportID, "reportId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.BLDPayload(args.ReportID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("bsd", "bsd", "Request an espionage report by inbox message ID.", EffectGameQuery,
		objectSchema(map[string]interface{}{"messageId": schemaProperty("integer", "Spy report inbox MID.")}, "messageId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[reportMessageIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive64(args.MessageID, "messageId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.BSDPayload(args.MessageID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ssi", "ssi", "Request live spy information for a target map tile.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"targetX":   schemaProperty("integer", "Target X coordinate."),
			"targetY":   schemaProperty("integer", "Target Y coordinate."),
			"kingdomId": schemaProperty("integer", "Target kingdom KID."),
		}, "targetX", "targetY", "kingdomId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[spyInfoArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.TargetX < 0 || args.TargetY < 0 || args.KingdomID < 0 {
				return nil, toolError("invalid_arguments", "target coordinates and kingdomId must not be negative")
			}
			return onePayload(GameCommands.SSIPayload(args.TargetX, args.TargetY, args.KingdomID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("mfs", "mfs", "Forward an inbox report to selected player IDs.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"messageId": schemaProperty("integer", "Inbox message MID."),
			"playerIds": map[string]interface{}{
				"type":        "array",
				"description": "One or more recipient player OIDs.",
				"items":       schemaProperty("integer", "Recipient player OID."),
				"minItems":    1,
			},
		}, "messageId", "playerIds"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[shareReportArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive64(args.MessageID, "messageId"); err != nil {
				return nil, err
			}
			if len(args.PlayerIDs) == 0 {
				return nil, toolError("invalid_arguments", "playerIds must not be empty")
			}
			for _, playerID := range args.PlayerIDs {
				if playerID <= 0 {
					return nil, toolError("invalid_arguments", "playerIds must contain only positive IDs")
				}
			}
			return onePayload(GameCommands.MFSPayload(args.MessageID, args.PlayerIDs)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("custom_empty", "dynamic", "Send an empty-body EmpireEx_21 debug command. This is intentionally classified as destructive because the opcode is dynamic.", EffectDestructive,
		objectSchema(map[string]interface{}{"opcode": schemaProperty("string", "Lowercase ASCII command opcode, 2 to 12 characters.")}, "opcode"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[customEmptyArgs](raw)
			if err != nil {
				return nil, err
			}
			opcode := strings.TrimSpace(args.Opcode)
			if !validCommandOpcode(opcode) {
				return nil, toolError("invalid_arguments", "opcode must contain 2 to 12 lowercase ASCII letters or digits")
			}
			// SendEmpireEx21EmptyCommand does not expose its pure builder. Reuse a known
			// empty frame and replace only the validated opcode segment.
			payload := strings.Replace(GameCommands.GAMPayload(), "%gam%", "%"+opcode+"%", 1)
			return onePayload(payload), nil
		}); err != nil {
		return err
	}

	return nil
}

func validCommandOpcode(opcode string) bool {
	if len(opcode) < 2 || len(opcode) > 12 {
		return false
	}
	for _, r := range opcode {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
