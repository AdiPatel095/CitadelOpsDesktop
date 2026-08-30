package GameData

import "strconv"

// AllianceHelpMaximumHelpers returns the official completion threshold for an
// alliance-help request type. Callers fail closed when the live catalog does
// not contain a usable threshold.
func (store *Store) AllianceHelpMaximumHelpers(requestType int) (int, bool) {
	if store == nil || requestType <= 0 {
		return 0, false
	}
	catalog, err := store.Catalog("alliancehelprequests")
	if err != nil {
		return 0, false
	}
	raw, found := catalog.FindByField("allianceHelpRequestID", strconv.Itoa(requestType))
	if !found {
		return 0, false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	maximum, found := record.Int64("maxHelpersCount")
	if !found || maximum <= 0 {
		return 0, false
	}
	return int(maximum), true
}
