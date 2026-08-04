package GameData

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"CitadelDesktop/Server/State"
)

type FoodConsumptionRate struct {
	ResourceID              State.ResourceID `json:"resourceId"`
	ResourceJSONKey         string           `json:"resourceJsonKey"`
	BasePerHour             float64          `json:"basePerHour"`
	ConsumptionMultiplier   float64          `json:"consumptionMultiplier"`
	MultiplierSource        string           `json:"multiplierSource"`
	CalculatedPerHour       float64          `json:"calculatedPerHour"`
	ObservedPerHour         *float64         `json:"observedPerHour,omitempty"`
	EffectivePerHour        float64          `json:"effectivePerHour"`
	ProductionInputPerHour  float64          `json:"productionInputPerHour"`
	TotalConsumptionPerHour float64          `json:"totalConsumptionPerHour"`
	ProductionPerHour       *float64         `json:"productionPerHour,omitempty"`
	NetPerHour              *float64         `json:"netPerHour,omitempty"`
}

type CastleFoodConsumption struct {
	CastleID       State.CastleID                           `json:"castleId"`
	ByResource     map[State.ResourceID]FoodConsumptionRate `json:"byResource"`
	MeadProduction MeadProductionDependency                 `json:"meadProduction"`
}

type MeadProductionDependency struct {
	ProductionPerHour            float64  `json:"productionPerHour"`
	HoneyInputPerHour            float64  `json:"honeyInputPerHour"`
	FoodInputPerHour             float64  `json:"foodInputPerHour"`
	SustainableProductionPerHour *float64 `json:"sustainableProductionPerHour,omitempty"`
	HoneyHoursUntilDepleted      *float64 `json:"honeyHoursUntilDepleted,omitempty"`
	FoodHoursUntilDepleted       *float64 `json:"foodHoursUntilDepleted,omitempty"`
}

type foodConsumptionDefinition struct {
	resourceJSONKey string
	supplyField     string
	reductionField  string
}

var foodConsumptionDefinitions = []foodConsumptionDefinition{
	{resourceJSONKey: "F", supplyField: "foodSupply", reductionField: "Foodreduction"},
	{resourceJSONKey: "MEAD", supplyField: "meadSupply", reductionField: "Meadreduction"},
	{resourceJSONKey: "BEEF", supplyField: "beefSupply", reductionField: "Beefreduction"},
}

var foodResourceJSONKeys = []string{"F", "HONEY", "MEAD", "BEEF"}

func (store *Store) UnitUsesFoodSupply(unitID State.UnitID) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("official game data is unavailable")
	}
	if unitID <= 0 {
		return false, fmt.Errorf("unit ID must be positive")
	}
	units, err := store.Catalog("units")
	if err != nil {
		return false, err
	}
	record, err := catalogRecord(units, int64(unitID), "unit")
	if err != nil {
		return false, err
	}
	foodSupply, _ := record.Float64("foodSupply")
	meadSupply, _ := record.Float64("meadSupply")
	beefSupply, _ := record.Float64("beefSupply")
	if meadSupply > 0 || beefSupply > 0 {
		return false, nil
	}
	if foodSupply <= 0 {
		return false, fmt.Errorf("official provision type is unavailable for unit %d", unitID)
	}
	return true, nil
}

// EstimateFoodConsumption calculates troop provisions and the food and honey
// inputs required by the configured brewery rate. The game-supplied consumption
// multiplier and observed troop rate are preferred when available because they
// include temporary bonuses. If those values have not been observed yet,
// reductions from current buildings and construction items are used as the
// fallback.
func (store *Store) EstimateFoodConsumption(castle State.CastleState) (CastleFoodConsumption, error) {
	if store == nil {
		return CastleFoodConsumption{}, fmt.Errorf("official game data is unavailable")
	}
	units, err := store.Catalog("units")
	if err != nil {
		return CastleFoodConsumption{}, err
	}
	resourceIDs, err := foodResourceIDs(store)
	if err != nil {
		return CastleFoodConsumption{}, err
	}
	baseByKey, err := foodConsumptionBase(castle.Units.Stationed, units)
	if err != nil {
		return CastleFoodConsumption{}, err
	}
	reductionByKey, err := foodConsumptionReductions(store, castle)
	if err != nil {
		return CastleFoodConsumption{}, err
	}
	result := CastleFoodConsumption{
		CastleID:   castle.ID,
		ByResource: make(map[State.ResourceID]FoodConsumptionRate, len(foodConsumptionDefinitions)),
	}
	for _, definition := range foodConsumptionDefinitions {
		resourceID := resourceIDs[definition.resourceJSONKey]
		balance := castle.Resources[resourceID]
		multiplier := 1 - reductionByKey[definition.resourceJSONKey]/100
		source := "catalog"
		if balance.ConsumptionMultiplier != nil {
			multiplier = *balance.ConsumptionMultiplier
			source = "observed"
		}
		multiplier = math.Max(0, multiplier)
		calculated := baseByKey[definition.resourceJSONKey] * multiplier
		effective := calculated
		if balance.ConsumptionPerHour != nil {
			effective = *balance.ConsumptionPerHour
		}
		rate := FoodConsumptionRate{
			ResourceID:            resourceID,
			ResourceJSONKey:       definition.resourceJSONKey,
			BasePerHour:           baseByKey[definition.resourceJSONKey],
			ConsumptionMultiplier: multiplier,
			MultiplierSource:      source,
			CalculatedPerHour:     calculated,
			ObservedPerHour:       cloneFloatPointer(balance.ConsumptionPerHour),
			EffectivePerHour:      effective,
			ProductionPerHour:     cloneFloatPointer(balance.ProductionPerHour),
		}
		setFoodConsumptionNet(&rate)
		result.ByResource[resourceID] = rate
	}
	honeyID := resourceIDs["HONEY"]
	honeyBalance := castle.Resources[honeyID]
	honeyRate := FoodConsumptionRate{
		ResourceID:            honeyID,
		ResourceJSONKey:       "HONEY",
		ConsumptionMultiplier: 1,
		MultiplierSource:      "not-applicable",
		ProductionPerHour:     cloneFloatPointer(honeyBalance.ProductionPerHour),
	}
	setFoodConsumptionNet(&honeyRate)
	result.ByResource[honeyID] = honeyRate
	result.MeadProduction, err = applyMeadProductionInputs(store, castle, resourceIDs, &result)
	if err != nil {
		return CastleFoodConsumption{}, err
	}
	return result, nil
}

func foodResourceIDs(store *Store) (map[string]State.ResourceID, error) {
	resources, err := store.Catalog("resources")
	if err != nil {
		return nil, err
	}
	result := map[string]State.ResourceID{}
	for _, raw := range resources.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, idExists := record.Int64("resourceID")
		jsonKey, keyExists := record.String("JSONKey")
		if idExists && id > 0 && keyExists {
			result[jsonKey] = State.ResourceID(id)
		}
	}
	for _, jsonKey := range foodResourceJSONKeys {
		if result[jsonKey] == 0 {
			return nil, fmt.Errorf("official resource %s is unavailable", jsonKey)
		}
	}
	return result, nil
}

func applyMeadProductionInputs(
	store *Store,
	castle State.CastleState,
	resourceIDs map[string]State.ResourceID,
	result *CastleFoodConsumption,
) (MeadProductionDependency, error) {
	inputs, err := meadProductionInputs(store, castle)
	if err != nil {
		return MeadProductionDependency{}, err
	}
	if inputs.configuredProductionPerHour <= 0 {
		return MeadProductionDependency{}, nil
	}
	meadRate := result.ByResource[resourceIDs["MEAD"]]
	productionPerHour := inputs.configuredProductionPerHour
	if meadRate.ProductionPerHour != nil {
		productionPerHour = *meadRate.ProductionPerHour
	}
	honeyInput := inputs.honeyInputPerHour
	foodInput := inputs.foodInputPerHour

	foodID := resourceIDs["F"]
	foodRate := result.ByResource[foodID]
	foodRate.ProductionInputPerHour = foodInput
	foodRate.TotalConsumptionPerHour = foodRate.EffectivePerHour + foodInput
	setFoodConsumptionNet(&foodRate)
	result.ByResource[foodID] = foodRate

	honeyID := resourceIDs["HONEY"]
	honeyRate := result.ByResource[honeyID]
	honeyRate.ProductionInputPerHour = honeyInput
	honeyRate.TotalConsumptionPerHour = honeyRate.EffectivePerHour + honeyInput
	setFoodConsumptionNet(&honeyRate)
	result.ByResource[honeyID] = honeyRate

	dependency := MeadProductionDependency{
		ProductionPerHour:       productionPerHour,
		HoneyInputPerHour:       honeyInput,
		FoodInputPerHour:        foodInput,
		HoneyHoursUntilDepleted: depletionHours(castle.Resources[honeyID].Amount, honeyRate.NetPerHour, honeyInput),
		FoodHoursUntilDepleted:  depletionHours(castle.Resources[foodID].Amount, foodRate.NetPerHour, foodInput),
	}
	if sustainable, known := sustainableMeadProduction(productionPerHour, foodInput, honeyInput, foodRate.ProductionPerHour, honeyRate.ProductionPerHour); known {
		dependency.SustainableProductionPerHour = &sustainable
	}
	return dependency, nil
}

type meadInputs struct {
	configuredProductionPerHour float64
	honeyInputPerHour           float64
	foodInputPerHour            float64
}

func meadProductionInputs(store *Store, castle State.CastleState) (meadInputs, error) {
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return meadInputs{}, err
	}
	result := meadInputs{}
	for _, building := range castle.Buildings {
		record, recordErr := catalogRecord(buildings, int64(building.DefinitionID), "building")
		if recordErr != nil {
			return meadInputs{}, recordErr
		}
		production, exists := record.Float64("meadProduction")
		if !exists || production <= 0 {
			continue
		}
		buildingProduction := castle.BuildingProduction[building.InstanceID]
		percent, observed := buildingProduction.PercentByResource["MEAD"]
		if !observed {
			return meadInputs{}, fmt.Errorf("brewery production percentage is unavailable for building %d", building.InstanceID)
		}
		if percent < 0 || percent > 100 {
			return meadInputs{}, fmt.Errorf("brewery production percentage %.2f is invalid for building %d", percent, building.InstanceID)
		}
		honeyRatio, _ := record.Float64("honeyRatio")
		foodRatio, _ := record.Float64("foodRatio")
		// The catalog rate is stored in hundredths; PA.MEAD is the configured
		// operating percentage. Live DMEAD output does not determine inputs.
		configuredRate := production / 100 * percent / 100
		result.configuredProductionPerHour += configuredRate
		result.honeyInputPerHour += configuredRate * honeyRatio
		result.foodInputPerHour += configuredRate * foodRatio
	}
	return result, nil
}

func setFoodConsumptionNet(rate *FoodConsumptionRate) {
	if rate == nil {
		return
	}
	if rate.TotalConsumptionPerHour == 0 {
		rate.TotalConsumptionPerHour = rate.EffectivePerHour
	}
	if rate.ProductionPerHour == nil {
		rate.NetPerHour = nil
		return
	}
	net := *rate.ProductionPerHour - rate.TotalConsumptionPerHour
	rate.NetPerHour = &net
}

func depletionHours(amount float64, net *float64, requiredPerHour float64) *float64 {
	if requiredPerHour <= 0 || net == nil || *net >= 0 {
		return nil
	}
	hours := math.Max(0, amount) / -*net
	return &hours
}

func sustainableMeadProduction(
	productionPerHour float64,
	foodInputPerHour float64,
	honeyInputPerHour float64,
	foodProduction *float64,
	honeyProduction *float64,
) (float64, bool) {
	if foodProduction == nil || honeyProduction == nil {
		return 0, false
	}
	sustainable := productionPerHour
	if foodInputPerHour > 0 {
		sustainable = math.Min(sustainable, productionPerHour*math.Max(0, *foodProduction)/foodInputPerHour)
	}
	if honeyInputPerHour > 0 {
		sustainable = math.Min(sustainable, productionPerHour*math.Max(0, *honeyProduction)/honeyInputPerHour)
	}
	return sustainable, true
}

func foodConsumptionBase(stationed map[State.UnitID]int64, units *Catalog) (map[string]float64, error) {
	result := map[string]float64{}
	missing := make([]int64, 0)
	for unitID, amount := range stationed {
		if amount <= 0 {
			continue
		}
		raw, exists := units.Find(strconv.FormatInt(int64(unitID), 10))
		if !exists {
			missing = append(missing, int64(unitID))
			continue
		}
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			missing = append(missing, int64(unitID))
			continue
		}
		for _, definition := range foodConsumptionDefinitions {
			if supply, exists := record.Float64(definition.supplyField); exists && supply > 0 {
				result[definition.resourceJSONKey] += float64(amount) * supply
			}
		}
	}
	if len(missing) > 0 {
		sort.Slice(missing, func(left, right int) bool { return missing[left] < missing[right] })
		return nil, fmt.Errorf("official unit definitions are unavailable for stationed units %v", missing)
	}
	return result, nil
}

func foodConsumptionReductions(store *Store, castle State.CastleState) (map[string]float64, error) {
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return nil, err
	}
	result := map[string]float64{}
	for _, building := range castle.Buildings {
		record, err := catalogRecord(buildings, int64(building.DefinitionID), "building")
		if err != nil {
			return nil, err
		}
		addFoodConsumptionReductions(result, record)
	}
	if len(castle.ConstructionSlots) == 0 {
		return result, nil
	}
	items, err := store.Catalog("constructionItems")
	if err != nil {
		return nil, err
	}
	for _, slots := range castle.ConstructionSlots {
		for _, slot := range slots {
			if slot.DefinitionID <= 0 {
				continue
			}
			record, recordErr := catalogRecord(items, int64(slot.DefinitionID), "construction item")
			if recordErr != nil {
				return nil, recordErr
			}
			addFoodConsumptionReductions(result, record)
		}
	}
	return result, nil
}

func catalogRecord(catalog *Catalog, id int64, kind string) (Record, error) {
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return nil, fmt.Errorf("official %s definition %d is unavailable", kind, id)
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return nil, fmt.Errorf("decode official %s definition %d: %w", kind, id, err)
	}
	return record, nil
}

func addFoodConsumptionReductions(target map[string]float64, record Record) {
	for _, definition := range foodConsumptionDefinitions {
		if reduction, exists := record.Float64(definition.reductionField); exists && reduction > 0 {
			target[definition.resourceJSONKey] += reduction
		}
	}
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
