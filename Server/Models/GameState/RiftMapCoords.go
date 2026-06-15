package gamestate

import mapstate "CitadelDesktop/Server/Models/MapState"

// RiftMapCoordsPayload builds the websocket payload for the Rift coord panel from MapState + castle focus.
func RiftMapCoordsPayload() map[string]interface{} {
	gs := GetGameState()
	f := gs.CastleFocus
	centerX, centerY := f.MapPX, f.MapPY
	if centerX == 0 && centerY == 0 {
		if x, y, ok := gs.ResolveCastleMapCoords(f.CastleAID, f.KingdomID); ok {
			centerX, centerY = x, y
		}
	}

	ms := mapstate.GetMapState()
	riftNode, riftKid, found := ms.FindRift()

	out := map[string]interface{}{
		"castleAid":     f.CastleAID,
		"kingdomID":     f.KingdomID,
		"centerX":       centerX,
		"centerY":       centerY,
		"riftKingdomID": 0,
		"found":         found,
		"rift":          nil,
		"deltaX":        0,
		"deltaY":        0,
		"distance":      0,
	}
	if !found {
		return out
	}

	wire := mapstate.MapNodeToWire(riftNode)
	out["rift"] = wire
	out["riftKingdomID"] = riftKid
	out["deltaX"] = riftNode.X - centerX
	out["deltaY"] = riftNode.Y - centerY
	if centerX != 0 || centerY != 0 {
		out["distance"] = mapstate.ChebyshevDistance(centerX, centerY, riftNode.X, riftNode.Y)
	}
	return out
}
