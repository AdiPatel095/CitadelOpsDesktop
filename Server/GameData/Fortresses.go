package GameData

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	KingdomFortressMapTypeID      = 11
	DirewolfUnitID                = 277
	FortressRelicSpeedEffectID    = 2106
	FortressDailySpeedEffectID    = 426
	FortressDailyGlobalEffectID   = 2
	FortressDailyBoosterRubyCost  = 2500
	fortressTravelSpeedEffectType = 15
)

type KingdomFortressDefinition struct {
	KingdomID           int   `json:"kingdomId"`
	Level               int   `json:"level"`
	GlobalCooldownSec   int64 `json:"globalCooldownSec"`
	PersonalCooldownSec int64 `json:"personalCooldownSec"`
}

type FortressUnitDefinition struct {
	UnitID int64  `json:"unitId"`
	Type   string `json:"type"`
	Name   string `json:"name"`
}

type FortressRelicSpeedContract struct {
	RelicEffectID       int64   `json:"relicEffectId"`
	RelicCapID          int64   `json:"relicCapId"`
	RelicMaximumPercent float64 `json:"relicMaximumPercent"`
}

type FortressSpeedContract struct {
	FortressRelicSpeedContract
	DailyGlobalEffectID int64   `json:"dailyGlobalEffectId"`
	DailyEffectID       int64   `json:"dailyEffectId"`
	DailyBoostPercent   float64 `json:"dailyBoostPercent"`
}

func (store *Store) KingdomFortressDefinitions() ([]KingdomFortressDefinition, error) {
	if store == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := store.Catalog("bossdungeons")
	if err != nil {
		return nil, fmt.Errorf("official fortress catalog is unavailable: %w", err)
	}
	definitions := make([]KingdomFortressDefinition, 0, 3)
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		kingdomID, hasKingdom := record.Int64("kID")
		level, hasLevel := record.Int64("dungeonlevel")
		globalCooldown, hasGlobal := record.Int64("cooldownDelay")
		personalCooldown, hasPersonal := record.Int64("playerCooldownDelay")
		if !hasKingdom || kingdomID < 1 || kingdomID > 3 || !hasLevel || level <= 0 ||
			!hasGlobal || globalCooldown <= 0 || !hasPersonal || personalCooldown <= globalCooldown {
			continue
		}
		definitions = append(definitions, KingdomFortressDefinition{
			KingdomID: int(kingdomID), Level: int(level), GlobalCooldownSec: globalCooldown,
			PersonalCooldownSec: personalCooldown,
		})
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].KingdomID < definitions[right].KingdomID
	})
	if len(definitions) == 0 {
		return nil, fmt.Errorf("official fortress catalog contains no supported kingdom definitions")
	}
	return definitions, nil
}

func (store *Store) FortressDirewolf() (FortressUnitDefinition, error) {
	if store == nil {
		return FortressUnitDefinition{}, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := store.Catalog("units")
	if err != nil {
		return FortressUnitDefinition{}, fmt.Errorf("official unit catalog is unavailable: %w", err)
	}
	raw, found := catalog.Find(strconv.FormatInt(DirewolfUnitID, 10))
	if !found {
		return FortressUnitDefinition{}, fmt.Errorf("official Direwolf unit %d is unavailable", DirewolfUnitID)
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return FortressUnitDefinition{}, fmt.Errorf("decode official Direwolf unit: %w", err)
	}
	unitType := strings.TrimSpace(stringValue(record, "type"))
	comment := strings.TrimSpace(stringValue(record, "comment1"))
	if !strings.EqualFold(unitType, "Elitetinoswolves") || !strings.Contains(strings.ToLower(comment), "nomad") {
		return FortressUnitDefinition{}, fmt.Errorf("official unit %d no longer matches the Nomad Direwolf contract", DirewolfUnitID)
	}
	name := strings.TrimSpace(stringValue(record, "name"))
	if name == "" || strings.EqualFold(name, "Eventunit") {
		name = "Direwolf"
	}
	return FortressUnitDefinition{UnitID: DirewolfUnitID, Type: unitType, Name: name}, nil
}

// FortressRelicSpeed validates only the Relic 2.0 commander speed contract.
// Auto Fortress intentionally depends on this contract but not on the
// separately controlled premium global-effect booster.
func (store *Store) FortressRelicSpeed() (FortressRelicSpeedContract, error) {
	if store == nil {
		return FortressRelicSpeedContract{}, fmt.Errorf("official game data is unavailable")
	}
	effects, err := store.Catalog("effects")
	if err != nil {
		return FortressRelicSpeedContract{}, fmt.Errorf("official effect catalog is unavailable: %w", err)
	}
	relicRaw, found := effects.Find(strconv.FormatInt(FortressRelicSpeedEffectID, 10))
	if !found {
		return FortressRelicSpeedContract{}, fmt.Errorf("official Relic 2.0 travel-speed effect %d is unavailable", FortressRelicSpeedEffectID)
	}
	relic, err := DecodeRecord(relicRaw)
	if err != nil {
		return FortressRelicSpeedContract{}, fmt.Errorf("decode official Relic 2.0 travel-speed effect: %w", err)
	}
	relicType, hasRelicType := relic.Int64("effectTypeID")
	relicCapID, hasRelicCap := relic.Int64("capID")
	if !hasRelicType || relicType != fortressTravelSpeedEffectType || !hasRelicCap || relicCapID <= 0 ||
		!strings.EqualFold(strings.TrimSpace(stringValue(relic, "name")), "relicSpeedBonus") {
		return FortressRelicSpeedContract{}, fmt.Errorf("official Relic 2.0 travel-speed effect no longer matches the fortress contract")
	}
	caps, err := store.Catalog("effectCaps")
	if err != nil {
		return FortressRelicSpeedContract{}, fmt.Errorf("official effect-cap catalog is unavailable: %w", err)
	}
	capRaw, found := caps.Find(strconv.FormatInt(relicCapID, 10))
	if !found {
		return FortressRelicSpeedContract{}, fmt.Errorf("official Relic 2.0 travel-speed cap %d is unavailable", relicCapID)
	}
	capRecord, err := DecodeRecord(capRaw)
	if err != nil {
		return FortressRelicSpeedContract{}, fmt.Errorf("decode official Relic 2.0 travel-speed cap: %w", err)
	}
	maximum, hasMaximum := capRecord.Float64("maxTotalBonus")
	if !hasMaximum || maximum <= 0 || math.IsNaN(maximum) || math.IsInf(maximum, 0) {
		return FortressRelicSpeedContract{}, fmt.Errorf("official Relic 2.0 travel-speed cap is invalid")
	}
	return FortressRelicSpeedContract{
		RelicEffectID: FortressRelicSpeedEffectID, RelicCapID: relicCapID, RelicMaximumPercent: maximum,
	}, nil
}

// FortressSpeed validates the independent Relic 2.0 and global-effect speed
// identities. The fixed 2,500-ruby safety ceiling is not treated as a catalog
// quote: Auto Booster must still observe that exact price in the current SEI
// offer immediately before it sends the premium purchase.
func (store *Store) FortressSpeed() (FortressSpeedContract, error) {
	relic, err := store.FortressRelicSpeed()
	if err != nil {
		return FortressSpeedContract{}, err
	}
	effects, err := store.Catalog("effects")
	if err != nil {
		return FortressSpeedContract{}, fmt.Errorf("official effect catalog is unavailable: %w", err)
	}

	dailyRaw, found := effects.Find(strconv.FormatInt(FortressDailySpeedEffectID, 10))
	if !found {
		return FortressSpeedContract{}, fmt.Errorf("official daily fortress speed effect %d is unavailable", FortressDailySpeedEffectID)
	}
	daily, err := DecodeRecord(dailyRaw)
	if err != nil {
		return FortressSpeedContract{}, fmt.Errorf("decode official daily fortress speed effect: %w", err)
	}
	dailyType, hasDailyType := daily.Int64("effectTypeID")
	if !hasDailyType || dailyType != fortressTravelSpeedEffectType ||
		!strings.EqualFold(strings.TrimSpace(stringValue(daily, "name")), "speedBonus") {
		return FortressSpeedContract{}, fmt.Errorf("official daily fortress speed effect no longer matches the travel-speed contract")
	}
	globalEffects, err := store.Catalog("globalEffects")
	if err != nil {
		return FortressSpeedContract{}, fmt.Errorf("official global-effect catalog is unavailable: %w", err)
	}
	dailyBoost := 0.0
	for _, raw := range globalEffects.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		globalEffectID, ok := record.Int64("globalEffectID")
		if !ok || globalEffectID != FortressDailyGlobalEffectID {
			continue
		}
		encoded := strings.TrimSpace(stringValue(record, "effects"))
		for _, token := range strings.FieldsFunc(encoded, func(value rune) bool { return value == '#' || value == ',' }) {
			parts := strings.Split(strings.TrimSpace(token), "&")
			if len(parts) != 2 {
				continue
			}
			effectID, effectErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			value, valueErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if effectErr == nil && valueErr == nil && effectID == FortressDailySpeedEffectID && value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
				dailyBoost = value
				break
			}
		}
		if dailyBoost > 0 {
			break
		}
	}
	if dailyBoost <= 0 {
		return FortressSpeedContract{}, fmt.Errorf("official daily global fortress speed effect is unavailable")
	}
	return FortressSpeedContract{
		FortressRelicSpeedContract: relic,
		DailyGlobalEffectID:        FortressDailyGlobalEffectID, DailyEffectID: FortressDailySpeedEffectID, DailyBoostPercent: dailyBoost,
	}, nil
}

// FortressDirewolfPackages returns only stock-tracked 100-unit Nomad packages,
// ordered cheapest first. Active event routing and balances are still checked
// immediately before every purchase by the regular Auto Buyer intent.
func (store *Store) FortressDirewolfPackages() ([]AutoBuyerPackage, error) {
	if _, err := store.FortressDirewolf(); err != nil {
		return nil, err
	}
	catalog, err := store.AutoBuyerCatalog()
	if err != nil {
		return nil, err
	}
	packages := make([]AutoBuyerPackage, 0)
	for _, product := range catalog.Packages {
		if product.ShopID != AutoBuyerShopNomad || product.UnitID != DirewolfUnitID ||
			product.UnitAmount != 100 || product.Stock <= 0 || product.Price.Premium {
			continue
		}
		packages = append(packages, product)
	}
	sort.Slice(packages, func(left, right int) bool {
		if packages[left].Price.Amount != packages[right].Price.Amount {
			return packages[left].Price.Amount < packages[right].Price.Amount
		}
		if packages[left].SortOrder != packages[right].SortOrder {
			return packages[left].SortOrder < packages[right].SortOrder
		}
		return packages[left].PackageID < packages[right].PackageID
	})
	if len(packages) == 0 {
		return nil, fmt.Errorf("official Nomad shop contains no supported 100-Direwolf packages")
	}
	return packages, nil
}
