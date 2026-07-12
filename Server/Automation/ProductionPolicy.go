package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type ProductionPolicy struct {
	id            string
	enabledKey    string
	section       string
	lineID        int
	definitionKey string
}

type productionSettings struct {
	Mode             string                      `json:"mode"`
	CheckIntervalSec int                         `json:"checkIntervalSec"`
	GlobalItems      []productionTarget          `json:"globalItems"`
	Castles          map[string]productionCastle `json:"castles"`
}

type productionTarget struct {
	ID     int64 `json:"id"`
	Amount int64 `json:"amount,omitempty"`
}

type productionCastle struct {
	Enabled bool               `json:"enabled"`
	Items   []productionTarget `json:"items"`
}

func NewRecruitPolicy() *ProductionPolicy {
	return &ProductionPolicy{
		id: "autoRecruit", enabledKey: "recruit_troops", section: "automation.recruitTroops",
		lineID: 0, definitionKey: "unit",
	}
}

func NewToolPolicy() *ProductionPolicy {
	return &ProductionPolicy{
		id: "autoTool", enabledKey: "auto_tool", section: "automation.autoTool",
		lineID: 1, definitionKey: "tool",
	}
}

func (policy *ProductionPolicy) ID() string { return policy.id }

func (policy *ProductionPolicy) EnabledKey() string { return policy.enabledKey }

func (policy *ProductionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := productionSettings{Mode: "global", CheckIntervalSec: 300, Castles: map[string]productionCastle{}}
	if !decodeSection(snapshot.Configuration, policy.section, &settings) {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("No %s production plan is configured", policy.definitionKey),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 300)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 300)
	if snapshot.State.CommandContext.ProductionSessionKey <= 0 {
		return Decision{
			Status:      "blocked",
			Detail:      fmt.Sprintf("Waiting to learn the live production session key; enqueue one %s stack in-game once", policy.definitionKey),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	configured := 0
	observed := 0
	full := 0
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		castlePlan := settings.Castles[castleKey]
		if !castlePlan.Enabled {
			continue
		}
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists {
			continue
		}
		targets := castlePlan.Items
		if settings.Mode != "perCastle" {
			targets = settings.GlobalItems
		}
		if len(targets) == 0 || targets[0].ID <= 0 {
			continue
		}
		configured++
		if allowed, _ := scheduleAllows(snapshot.Configuration, policy.id+":"+castleKey, snapshot.Now); !allowed && settings.Mode == "perCastle" {
			continue
		}
		queue, queueExists := castle.Production[policy.lineID]
		if !queueExists || queue.ObservedAt.IsZero() {
			continue
		}
		observed++
		if queue.Capacity <= 0 || len(queue.Queued) >= queue.Capacity {
			full++
			continue
		}
		target := targets[0]
		arguments, _ := json.Marshal(map[string]any{
			"castleId": castleID, "lineId": policy.lineID,
			"definitionId": target.ID, "amount": target.Amount,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Queue the configured %s at %s", policy.definitionKey, castleName(castle)),
			NextCheckAt: snapshot.Now.Add(interval),
			Request:     &Intent.Request{Name: "production.enqueue", Arguments: arguments},
		}, nil
	}
	detail := fmt.Sprintf("No enabled castle has a configured %s", policy.definitionKey)
	if configured > 0 && observed == 0 {
		detail = "Waiting for production queues to be observed in the game session"
	} else if configured > 0 && observed == full {
		detail = "All observed production queues are full"
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: snapshot.Now.Add(interval)}, nil
}

func castleName(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	return fmt.Sprintf("castle %d", castle.ID)
}
