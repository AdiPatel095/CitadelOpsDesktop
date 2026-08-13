package GameData

import (
	"strconv"
	"strings"
)

const gloryTitleUnitUnlockEffectID int64 = 46

// GloryTitleUnitUnlock describes a title-gated level-11 unit and the
// corresponding level-10 unit from the same official unit family.
type GloryTitleUnitUnlock struct {
	UnitID          int64
	RequiredTitleID int64
	Level10UnitID   int64
}

// GloryTitleFromDisplayIDs resolves the player's active glory title from the
// PRE/SUF fields reported by the game. The title catalog determines which of
// the displayed titles is the FAME (glory) title.
func (store *Store) GloryTitleFromDisplayIDs(prefixID int64, suffixID int64) (int64, bool) {
	return store.playerTitleFromDisplayIDs(prefixID, suffixID, "FAME")
}

// GallantryTitleFromDisplayIDs resolves the player's active gallantry title
// from the same PRE/SUF pair. The official catalog identifies gallantry titles
// as FACTION, independently from the FAME title used by Auto Recruit.
func (store *Store) GallantryTitleFromDisplayIDs(prefixID int64, suffixID int64) (int64, bool) {
	return store.playerTitleFromDisplayIDs(prefixID, suffixID, "FACTION")
}

func (store *Store) playerTitleFromDisplayIDs(prefixID int64, suffixID int64, wantedType string) (int64, bool) {
	if store == nil {
		return 0, false
	}
	titles, err := store.Catalog("titles")
	if err != nil {
		return 0, false
	}
	candidates := []struct {
		id      int64
		display string
	}{
		{id: prefixID, display: "prefix"},
		{id: suffixID, display: "suffix"},
	}
	resolved := int64(0)
	found := false
	for _, candidate := range candidates {
		raw, exists := titles.Find(strconv.FormatInt(candidate.id, 10))
		if !exists {
			continue
		}
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		recordType, _ := record.String("type")
		displayType, _ := record.String("displayType")
		if !strings.EqualFold(strings.TrimSpace(recordType), strings.TrimSpace(wantedType)) ||
			!strings.EqualFold(strings.TrimSpace(displayType), candidate.display) {
			continue
		}
		if found && resolved != candidate.id {
			return 0, false
		}
		resolved = candidate.id
		found = true
	}
	return resolved, found
}

// GloryTitleIncludes reports whether currentTitleID is the required glory
// title or one of its descendants in the official previousTitleID chain.
func (store *Store) GloryTitleIncludes(currentTitleID int64, requiredTitleID int64) bool {
	if store == nil || currentTitleID < 0 || requiredTitleID < 0 {
		return false
	}
	titles, err := store.Catalog("titles")
	if err != nil {
		return false
	}
	const maximumTitleChain = 128
	seen := map[int64]struct{}{}
	for current := currentTitleID; len(seen) < maximumTitleChain; {
		if _, duplicate := seen[current]; duplicate {
			return false
		}
		seen[current] = struct{}{}
		raw, exists := titles.Find(strconv.FormatInt(current, 10))
		if !exists {
			return false
		}
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			return false
		}
		titleType, _ := record.String("type")
		if !strings.EqualFold(strings.TrimSpace(titleType), "FAME") {
			return false
		}
		if current == requiredTitleID {
			return true
		}
		previous, exists := record.Int64("previousTitleID")
		if !exists || previous < 0 {
			return false
		}
		current = previous
	}
	return false
}

// GloryTitleUnlockForUnit reads the official title effect instead of relying
// on localized unit names or fixed title thresholds. Only a level-11 unit
// explicitly unlocked by a FAME title is returned.
func (store *Store) GloryTitleUnlockForUnit(unitID int64) (GloryTitleUnitUnlock, bool) {
	if store == nil || unitID <= 0 {
		return GloryTitleUnitUnlock{}, false
	}
	units, err := store.Catalog("units")
	if err != nil {
		return GloryTitleUnitUnlock{}, false
	}
	rawUnit, exists := units.Find(strconv.FormatInt(unitID, 10))
	if !exists {
		return GloryTitleUnitUnlock{}, false
	}
	unit, err := DecodeRecord(rawUnit)
	if err != nil {
		return GloryTitleUnitUnlock{}, false
	}
	level, levelExists := unit.Int64("level")
	unitType, typeExists := unit.String("type")
	if !levelExists || level != 11 || !typeExists || strings.TrimSpace(unitType) == "" {
		return GloryTitleUnitUnlock{}, false
	}

	titles, err := store.Catalog("titles")
	if err != nil {
		return GloryTitleUnitUnlock{}, false
	}
	requiredTitleID := int64(-1)
	for _, raw := range titles.Rows() {
		title, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		titleType, _ := title.String("type")
		if !strings.EqualFold(strings.TrimSpace(titleType), "FAME") ||
			!recordUnlocksUnit(title, unitID) {
			continue
		}
		candidate, candidateExists := title.Int64("titleID")
		if !candidateExists || candidate < 0 || requiredTitleID >= 0 && requiredTitleID != candidate {
			return GloryTitleUnitUnlock{}, false
		}
		requiredTitleID = candidate
	}
	if requiredTitleID < 0 {
		return GloryTitleUnitUnlock{}, false
	}

	level10UnitID := int64(0)
	for _, raw := range units.Rows() {
		candidate, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidateType, _ := candidate.String("type")
		candidateLevel, candidateLevelExists := candidate.Int64("level")
		if !candidateLevelExists || candidateLevel != 10 ||
			!strings.EqualFold(strings.TrimSpace(candidateType), strings.TrimSpace(unitType)) {
			continue
		}
		candidateID, candidateIDExists := candidate.Int64("wodID")
		if !candidateIDExists || candidateID <= 0 || level10UnitID > 0 && level10UnitID != candidateID {
			return GloryTitleUnitUnlock{}, false
		}
		level10UnitID = candidateID
	}

	return GloryTitleUnitUnlock{
		UnitID: unitID, RequiredTitleID: requiredTitleID, Level10UnitID: level10UnitID,
	}, true
}

func recordUnlocksUnit(record Record, unitID int64) bool {
	effects, exists := record.String("effects")
	if !exists {
		return false
	}
	for _, effect := range strings.Split(effects, ",") {
		parts := strings.Split(strings.TrimSpace(effect), "&")
		if len(parts) != 2 {
			continue
		}
		effectID, effectErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		value, valueErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if effectErr == nil && valueErr == nil && effectID == gloryTitleUnitUnlockEffectID && value == unitID {
			return true
		}
	}
	return false
}
