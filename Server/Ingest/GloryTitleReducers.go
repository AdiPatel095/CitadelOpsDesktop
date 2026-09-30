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
	owners, err := playerTitleOwners(frame)
	if err != nil {
		return nil, false, err
	}
	for _, owner := range owners {
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

func playerTitleOwnersFromPayload(raw json.RawMessage) ([]playerTitleOwnerWire, error) {
	var payload struct {
		Owners []playerTitleOwnerWire `json:"O"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode movement owners for player titles: %w", err)
	}
	return payload.Owners, nil
}

func playerTitleOwners(frame Protocol.Frame) ([]playerTitleOwnerWire, error) {
	root, err := frame.PayloadRoot()
	if err != nil || Protocol.HasCaseFoldedAlias(root, "O") {
		return playerTitleOwnersFromPayload(frame.Payload)
	}
	var owners []playerTitleOwnerWire
	if raw := root["O"]; len(raw) > 0 {
		if json.Unmarshal(raw, &owners) != nil {
			return playerTitleOwnersFromPayload(frame.Payload)
		}
	}
	return owners, nil
}
