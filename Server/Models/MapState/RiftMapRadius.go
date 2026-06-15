package mapstate

import "sort"

// GaaNodeRiftType is gaa AI[0] for the single world Rift POI (len-18 row, KID 0).
const GaaNodeRiftType = 43

// RiftMapRadius is the Chebyshev tile radius around the focused castle for GAA map nodes.
const RiftMapRadius = 20

// RiftMapNodeWire is the slim shape pushed to the Rift view (no RawData).
type RiftMapNodeWire struct {
	Type            int    `json:"type"`
	TypeLabel       string `json:"typeLabel,omitempty"`
	X               int    `json:"x"`
	Y               int    `json:"y"`
	Name            string `json:"name,omitempty"`
	Level           int    `json:"level,omitempty"`
	CastleID        int    `json:"castleId,omitempty"`
	PlayerID        int    `json:"playerId,omitempty"`
	CooldownSeconds int    `json:"cooldownSec,omitempty"`
	LastHitter      string `json:"lastHitter,omitempty"`
}

func mapNodeToWire(n MapNode) RiftMapNodeWire {
	return MapNodeToWire(n)
}

// MapNodeToWire converts a MapNode to the JSON shape for the Rift view.
func MapNodeToWire(n MapNode) RiftMapNodeWire {
	return RiftMapNodeWire{
		Type:            n.Type,
		TypeLabel:       LabelGaaNodeType(n.Type),
		X:               n.X,
		Y:               n.Y,
		Name:            n.Name,
		Level:           n.Level,
		CastleID:        n.CastleID,
		PlayerID:        n.PlayerID,
		CooldownSeconds: n.CooldownSeconds,
		LastHitter:      n.LastHitter,
	}
}

// FindRift returns the sole Rift map node if known (prefers KID 0). Only one Rift exists on the map.
func (ms *MapState) FindRift() (MapNode, int, bool) {
	if ms == nil {
		return MapNode{}, 0, false
	}
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	kids := make([]int, 0, len(ms.Kingdoms))
	for kid := range ms.Kingdoms {
		kids = append(kids, kid)
	}
	sort.Ints(kids)
	for pass := 0; pass < 2; pass++ {
		for _, kid := range kids {
			if pass == 0 && kid != 0 {
				continue
			}
			tiles := ms.Kingdoms[kid]
			for _, n := range tiles {
				if n.Type == GaaNodeRiftType {
					return n, kid, true
				}
			}
		}
	}
	return MapNode{}, 0, false
}

// ChebyshevDistance is max(|dx|, |dy|) between two map tiles.
func ChebyshevDistance(x1, y1, x2, y2 int) int {
	dx := x1 - x2
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y2
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

func chebyshevWithinRadius(cx, cy, x, y, radius int) bool {
	dx := x - cx
	if dx < 0 {
		dx = -dx
	}
	dy := y - cy
	if dy < 0 {
		dy = -dy
	}
	if dx > radius {
		return false
	}
	return dy <= radius
}

// NodesWithinRadius returns map nodes for kid within Chebyshev distance radius of (centerX, centerY).
func (ms *MapState) NodesWithinRadius(kid, centerX, centerY, radius int) []MapNode {
	if ms == nil || radius < 0 {
		return nil
	}
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	tiles := ms.Kingdoms[kid]
	if len(tiles) == 0 {
		return nil
	}
	out := make([]MapNode, 0, len(tiles))
	for _, n := range tiles {
		if chebyshevWithinRadius(centerX, centerY, n.X, n.Y, radius) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// WireNodesWithinRadius returns JSON-safe nodes near (centerX, centerY).
func (ms *MapState) WireNodesWithinRadius(kid, centerX, centerY, radius int) []RiftMapNodeWire {
	nodes := ms.NodesWithinRadius(kid, centerX, centerY, radius)
	out := make([]RiftMapNodeWire, len(nodes))
	for i, n := range nodes {
		out[i] = mapNodeToWire(n)
	}
	return out
}
