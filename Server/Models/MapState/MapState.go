package mapstate

import (
	"fmt"
	"sync"
)

type MapNode struct {
	Type            int
	X               int
	Y               int
	ID              int // Original generic ID
	CastleID        int
	PlayerID        int // Owner/PlayerID
	Level           int // Tower level or current state
	KeepLevel       int
	WallLevel       int
	GateLevel       int
	MoatLevel       int
	CooldownSeconds int
	Name            string
	LastHitter      string
	RawData         []interface{}
}

type MapPlayer struct {
	OID          int    `json:"OID"`
	Name         string `json:"N"`
	Level        int    `json:"L"`
	ParagonLevel int    `json:"LL"`
	AllianceID   int    `json:"AID"`
	AllianceName string `json:"AN"`
	MaxPower     int    `json:"MP"`
	CastleForce  int    `json:"CF"`
	HighestForce int    `json:"HF"`
	Might        int    `json:"H"`
}

type MapState struct {
	mu       sync.RWMutex
	Kingdoms map[int]map[string]MapNode // KID -> (X_Y -> MapNode)
}

var (
	instanceMapState *MapState
	onceMapState     sync.Once
)

func GetMapState() *MapState {
	onceMapState.Do(func() {
		instanceMapState = &MapState{
			Kingdoms: make(map[int]map[string]MapNode),
		}
	})
	return instanceMapState
}

func (ms *MapState) Reset() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.Kingdoms = make(map[int]map[string]MapNode)
}

func (ms *MapState) AddNode(kid int, n MapNode) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.Kingdoms[kid] == nil {
		ms.Kingdoms[kid] = make(map[string]MapNode)
	}

	key := fmt.Sprintf("%d_%d", n.X, n.Y)
	ms.Kingdoms[kid][key] = n
}
