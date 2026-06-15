package GameParser

import "fmt"

// StormCastlePresetKey is the stable on-disk key for Storm castle decoration presets.
// Storm castle instance ids (AID) rotate each event month; layouts are keyed by slot instead.
const StormCastlePresetKey = "stormCastle"

// DecorationPresetStorageKey returns the JSON map key used in DecorationPresets.json.
// Rotating event castles use a stable slot id; all other castles use the numeric instance id.
func DecorationPresetStorageKey(castleID int) string {
	if slot := GetCastleLocationName(castleID); slot == StormCastlePresetKey {
		return StormCastlePresetKey
	}
	return fmt.Sprintf("%d", castleID)
}
