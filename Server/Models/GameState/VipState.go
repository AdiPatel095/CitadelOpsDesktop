package gamestate

// VipState stores the active VIP snapshot from gbd.vip.
type VipState struct {
	Points       int `json:"points,omitempty"`
	Level        int `json:"level,omitempty"`
	RemainingSec int `json:"remainingSec,omitempty"`
	Upgrade      int `json:"upgrade,omitempty"`
}
