package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type playerTitleOwnerWire struct {
	PlayerID wireInt64       `json:"OID"`
	PrefixID json.RawMessage `json:"PRE"`
	SuffixID json.RawMessage `json:"SUF"`
	TopX     json.RawMessage `json:"TOPX"`
	Glory    json.RawMessage `json:"CF"`
}

func reducePlayerTitles(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 || gameState.Player.ID <= 0 || gameData == nil {
		return nil, false, nil
	}
	var payload struct {
		Owners []playerTitleOwnerWire `json:"O"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode movement owners for player titles: %w", err)
	}
	for _, owner := range payload.Owners {
		if State.PlayerID(owner.PlayerID) != gameState.Player.ID {
			continue
		}
		prefixID, prefixFound := rawInt64(owner.PrefixID)
		suffixID, suffixFound := rawInt64(owner.SuffixID)
		if !prefixFound || !suffixFound {
			return nil, false, nil
		}
		gloryTitleID, gloryFound := gameData.GloryTitleFromDisplayIDs(prefixID, suffixID)
		gallantryTitleID, gallantryFound := gameData.GallantryTitleFromDisplayIDs(prefixID, suffixID)
		if !gloryFound && !gallantryFound {
			return nil, false, nil
		}
		topX := gameState.Player.GloryTitleTopX
		if value, valueFound := rawInt64(owner.TopX); valueFound {
			topX = int(value)
		}
		glory := gameState.Player.Glory
		if value, valueFound := rawFloat64(owner.Glory); valueFound {
			glory = value
		}
		observedAt := frame.ReceivedAt.UTC()
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		domains := []string{"player"}
		changed := false
		if gloryFound {
			gloryTitleChanged := gameState.Player.GloryTitleID != gloryTitleID ||
				gameState.Player.GloryTitleGen != gameState.Session.ConnectionGeneration ||
				gameState.Player.GloryTitleAt.IsZero()
			changed = gloryTitleChanged || gameState.Player.GloryTitleTopX != topX || gameState.Player.Glory != glory
			gameState.Player.GloryTitleID = gloryTitleID
			gameState.Player.GloryTitleTopX = topX
			gameState.Player.GloryTitleAt = observedAt
			gameState.Player.GloryTitleGen = gameState.Session.ConnectionGeneration
			gameState.Player.Glory = glory
			if gloryTitleChanged {
				domains = append(domains, "glory-title")
			}
		}
		if gallantryFound {
			gallantryTitleChanged := gameState.Player.GallantryTitleID != gallantryTitleID ||
				gameState.Player.GallantryTitleGen != gameState.Session.ConnectionGeneration ||
				gameState.Player.GallantryTitleAt.IsZero()
			changed = changed || gallantryTitleChanged
			gameState.Player.GallantryTitleID = gallantryTitleID
			gameState.Player.GallantryTitleAt = observedAt
			gameState.Player.GallantryTitleGen = gameState.Session.ConnectionGeneration
			if gallantryTitleChanged {
				domains = append(domains, "gallantry-title")
			}
		}
		if !changed {
			return nil, false, nil
		}
		return domains, true, nil
	}
	return nil, false, nil
}
