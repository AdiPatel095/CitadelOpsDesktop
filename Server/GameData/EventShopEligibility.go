package GameData

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ValidateEventShopDestination verifies the routing restrictions attached to
// one official event shop and package. Live package advertisement is runtime
// state and is checked by the caller separately.
func (store *Store) ValidateEventShopDestination(
	packageID int64,
	eventID int64,
	kingdomID int64,
	areaType int,
) error {
	if store == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	if packageID <= 0 || eventID <= 0 || kingdomID < 0 || areaType <= 0 {
		return fmt.Errorf("event shop route contains an invalid package, event, kingdom, or area type")
	}
	events, err := store.Catalog("events")
	if err != nil {
		return fmt.Errorf("load official event shops: %w", err)
	}
	rawEvent, found := events.Find(strconv.FormatInt(eventID, 10))
	if !found {
		return fmt.Errorf("event shop %d is absent from the official catalog", eventID)
	}
	event, err := DecodeRecord(rawEvent)
	if err != nil {
		return fmt.Errorf("decode official event shop %d: %w", eventID, err)
	}
	kingdoms, restricted, err := strictCatalogIDList(event, "kIDs", false)
	if err != nil {
		return fmt.Errorf("event shop %d kingdom restrictions are malformed: %w", eventID, err)
	}
	if restricted && !containsCatalogID(kingdoms, kingdomID) {
		return fmt.Errorf("event shop %d is not eligible in kingdom %d", eventID, kingdomID)
	}
	areas, restricted, err := strictCatalogIDList(event, "areaTypes", false)
	if err != nil {
		return fmt.Errorf("event shop %d area restrictions are malformed: %w", eventID, err)
	}
	if restricted && !containsCatalogID(areas, int64(areaType)) {
		return fmt.Errorf("event shop %d is not eligible for area type %d", eventID, areaType)
	}

	packages, err := store.Catalog("packages")
	if err != nil {
		return fmt.Errorf("load official packages: %w", err)
	}
	rawPackage, found := packages.Find(strconv.FormatInt(packageID, 10))
	if !found {
		return fmt.Errorf("package %d is absent from the official catalog", packageID)
	}
	pack, err := DecodeRecord(rawPackage)
	if err != nil {
		return fmt.Errorf("decode official package %d: %w", packageID, err)
	}
	excluded, restricted, err := strictCatalogIDList(pack, "excludedAreaTypes", true)
	if err != nil {
		return fmt.Errorf("package %d area exclusions are malformed: %w", packageID, err)
	}
	if restricted && containsCatalogID(excluded, int64(areaType)) {
		return fmt.Errorf("package %d excludes area type %d", packageID, areaType)
	}
	return nil
}

// EventShopPackageIDs returns the catalog-defined package membership that the
// official client merges with each active event's PID/PIDS additions.
func (store *Store) EventShopPackageIDs(eventID int64) ([]int64, error) {
	if store == nil || eventID <= 0 {
		return nil, fmt.Errorf("official event shop is unavailable")
	}
	events, err := store.Catalog("events")
	if err != nil {
		return nil, fmt.Errorf("load official event shops: %w", err)
	}
	rawEvent, found := events.Find(strconv.FormatInt(eventID, 10))
	if !found {
		return nil, fmt.Errorf("event shop %d is absent from the official catalog", eventID)
	}
	event, err := DecodeRecord(rawEvent)
	if err != nil {
		return nil, fmt.Errorf("decode official event shop %d: %w", eventID, err)
	}
	rawJSON, present := event["packageIDs"]
	if !present {
		return nil, nil
	}
	if string(bytes.TrimSpace(rawJSON)) == "null" {
		return nil, fmt.Errorf("event shop %d packageIDs must be a string", eventID)
	}
	var raw string
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, fmt.Errorf("event shop %d packageIDs must be a string", eventID)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(value rune) bool {
		return value == '+' || value == ',' || value == '#'
	})
	result := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		packageID, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || packageID <= 0 {
			return nil, fmt.Errorf("event shop %d has invalid package id %q", eventID, strings.TrimSpace(part))
		}
		if _, duplicate := seen[packageID]; duplicate {
			continue
		}
		seen[packageID] = struct{}{}
		result = append(result, packageID)
	}
	return result, nil
}

func strictCatalogIDList(record Record, field string, minusOneMeansEmpty bool) ([]int64, bool, error) {
	rawJSON, present := record[field]
	if !present {
		return nil, false, nil
	}
	if string(bytes.TrimSpace(rawJSON)) == "null" {
		return nil, true, fmt.Errorf("restriction must be a string")
	}
	var raw string
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, true, fmt.Errorf("restriction must be a string")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || minusOneMeansEmpty && raw == "-1" {
		return nil, false, nil
	}
	parts := strings.FieldsFunc(raw, func(value rune) bool {
		return value == ',' || value == '#'
	})
	if len(parts) == 0 {
		return nil, true, fmt.Errorf("empty restriction list")
	}
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || value < 0 {
			return nil, true, fmt.Errorf("invalid id %q", strings.TrimSpace(part))
		}
		result = append(result, value)
	}
	return result, true, nil
}

func containsCatalogID(values []int64, wanted int64) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
