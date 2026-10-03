package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"sort"
	"strconv"
)

const StormSettingsKey = "storm"

func CastleSettingsKey(castle State.CastleState) string {
	if castle.KingdomID == GameData.StormKingdomID {
		return StormSettingsKey
	}
	return strconv.FormatInt(int64(castle.ID), 10)
}

// Role settings win even when empty. Only a currently owned Storm ID can
// supply a legacy fallback; unknown retired IDs are never guessed.
func CastleSettingsEntry[T any](entries map[string]T, castle State.CastleState) (T, bool) {
	if entry, ok := entries[CastleSettingsKey(castle)]; ok {
		return entry, true
	}
	entry, ok := entries[strconv.FormatInt(int64(castle.ID), 10)]
	return entry, ok
}

type CastleEntryBinding[T any] struct {
	Key    string
	Castle State.CastleState
	Entry  T
}

func BoundCastleEntries[T any](entries map[string]T, state *State.GameState) []CastleEntryBinding[T] {
	bindings := make([]CastleEntryBinding[T], 0)
	if state == nil {
		return bindings
	}
	for _, castle := range state.Castles {
		if entry, ok := CastleSettingsEntry(entries, castle); ok {
			bindings = append(bindings, CastleEntryBinding[T]{CastleSettingsKey(castle), castle, entry})
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Castle.ID < bindings[j].Castle.ID })
	return bindings
}
