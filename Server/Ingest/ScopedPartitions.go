package Ingest

import (
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

var focusedCastleOpcodes = map[string]struct{}{
	"jaa": {}, "jca": {}, "gui": {}, "hru": {}, "hdu": {}, "rpc": {}, "ubc": {},
	"ebe": {}, "etc": {}, "ebu": {}, "emo": {}, "eup": {}, "edo": {}, "sob": {}, "fco": {}, "msb": {}, "ego": {}, "scl": {},
	"spl": {}, "bup": {}, "crin": {}, "crst": {}, "crun": {}, "crsk": {}, "crca": {},
	"dfc": {}, "dfw": {}, "dfk": {}, "dfm": {}, "mos": {}, "gbc": {}, "sbp": {},
}

func scopedPartitionsForFrame(
	frame Protocol.Frame,
	gameState State.GameState,
	domains []string,
) []State.PartitionKey {
	opcode := strings.ToLower(strings.TrimSpace(frame.Opcode))
	keys := make([]State.PartitionKey, 0, len(domains)+8)
	if containsIngestDomain(domains, "session-context") || opcode == "jaa" || opcode == "jca" ||
		protocolFocusSubcontextForFrame(frame) != State.FocusSubcontextUnknown {
		keys = append(keys, State.SessionPartition(gameState, State.CapabilitySessionContext))
	}
	if opcode == "gii" {
		keys = append(keys, State.AccountPartition(gameState, State.CapabilityConstructionInventory))
	}
	if opcode == "gbc" {
		if castleID := gameState.Inventory.ConstructionOffersCastleID; castleID > 0 {
			keys = append(keys, State.CastlePartition(gameState, State.CapabilityConstructionCommerce, castleID))
		} else {
			keys = append(keys, State.AccountPartition(gameState, State.CapabilityConstructionCommerce))
		}
	}
	if opcode == "gbd" {
		for castleID := range gameState.Castles {
			keys = append(keys, scopedCastleDomainPartitions(gameState, castleID, domains)...)
		}
	}
	if _, focused := focusedCastleOpcodes[opcode]; focused {
		if castleID, _, found := focusedCastle(&gameState); found {
			keys = append(keys, scopedCastleDomainPartitions(gameState, castleID, domains)...)
		}
	}
	if containsMapIngestDomain(domains) {
		for _, kingdomID := range scopedMapKingdoms(frame, gameState) {
			keys = append(keys, State.KingdomPartition(gameState, State.CapabilityWorldMap, kingdomID))
			if containsKingdomEventDomain(domains) {
				keys = append(keys, State.KingdomPartition(gameState, State.CapabilityEvents, kingdomID))
			}
		}
	}
	return keys
}

func containsMapIngestDomain(domains []string) bool {
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "map" || strings.HasPrefix(domain, "map-") || domain == "storm-scan-progress" {
			return true
		}
	}
	return false
}

func protocolFocusSubcontextForFrame(frame Protocol.Frame) State.FocusSubcontext {
	if frame.Direction != Protocol.DirectionInbound ||
		(frame.ResponseCode != nil && *frame.ResponseCode != 0) {
		return State.FocusSubcontextUnknown
	}
	switch strings.ToLower(strings.TrimSpace(frame.Opcode)) {
	case "gaa":
		return State.FocusSubcontextMap
	case "jaa", "jca":
		return State.FocusSubcontextCastle
	default:
		return State.FocusSubcontextUnknown
	}
}

func scopedCastleDomainPartitions(
	gameState State.GameState,
	castleID State.CastleID,
	domains []string,
) []State.PartitionKey {
	keys := make([]State.PartitionKey, 0, len(domains))
	for _, domain := range domains {
		capability := castleCapabilityForDomain(domain)
		if capability != "" {
			keys = append(keys, State.CastlePartition(gameState, capability, castleID))
		}
	}
	return keys
}

func castleCapabilityForDomain(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "castles":
		return State.CapabilityCastleDirectory
	case "resources", "market":
		return State.CapabilityEconomy
	case "units", "beri":
		return State.CapabilityGarrison
	case "defense", "khan":
		return State.CapabilityDefense
	case "buildings", "building-layout":
		return State.CapabilityBuildings
	case "building-queue", "building-construction":
		return State.CapabilityBuildingQueue
	case "construction-items":
		return State.CapabilityConstructionItems
	case "production":
		return State.CapabilityProduction
	case "crafting":
		return State.CapabilityCrafting
	default:
		return ""
	}
}

func scopedMapKingdoms(frame Protocol.Frame, gameState State.GameState) []State.KingdomID {
	payload := frame.Payload
	for range 2 {
		var root map[string]json.RawMessage
		if json.Unmarshal(payload, &root) != nil {
			break
		}
		if raw, found := root["KID"]; found {
			return []State.KingdomID{State.KingdomID(rawInteger(raw))}
		}
		nested, found := root["gaa"]
		if !found {
			break
		}
		payload = nested
	}
	return gameState.MapKingdomIDs()
}

func containsKingdomEventDomain(domains []string) bool {
	for _, domain := range domains {
		switch strings.ToLower(strings.TrimSpace(domain)) {
		case "tower-cooldowns", "nomad-camps", "invasion", "storm", "rift", "events", "event-scores":
			return true
		}
	}
	return false
}

func containsIngestDomain(domains []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, domain := range domains {
		if strings.ToLower(strings.TrimSpace(domain)) == wanted {
			return true
		}
	}
	return false
}
