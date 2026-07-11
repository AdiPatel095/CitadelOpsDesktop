package spyreport

import "testing"

func TestIsPlayerCastleTarget(t *testing.T) {
	capture := Capture{BSD: map[string]interface{}{
		"CID": float64(1942435),
		"OI":  map[string]interface{}{"OID": float64(1493924), "DUM": false},
	}}
	if !IsPlayerCastleTarget(capture) {
		t.Fatal("player-owned castle was not recognized")
	}
}

func TestIsPlayerCastleTargetRejectsNPCAndAmbiguousOwners(t *testing.T) {
	for name, owner := range map[string]map[string]interface{}{
		"npc":       {"OID": float64(1493924), "DUM": true},
		"ambiguous": {"OID": float64(1493924)},
	} {
		t.Run(name, func(t *testing.T) {
			capture := Capture{BSD: map[string]interface{}{"CID": float64(1942435), "OI": owner}}
			if IsPlayerCastleTarget(capture) {
				t.Fatal("non-player target was recognized as a player castle")
			}
		})
	}
}
