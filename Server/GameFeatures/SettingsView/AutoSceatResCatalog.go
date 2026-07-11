package settingsview

import (
	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const (
	autoSceatCaravanOverloaderBoosterID = 11
	autoSceatBaseCapacityPerBarrow      = 100
)

// AutoSceatResourceMeta describes a recipe input/output and its official game icon when available.
type AutoSceatResourceMeta struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	JSONKey   string `json:"jsonKey,omitempty"`
	AssetName string `json:"assetName,omitempty"`
	IconURL   string `json:"iconUrl,omitempty"`
}

// AutoSceatRecipeCatalogEntry is one official crafting recipe enriched with reward metadata.
type AutoSceatRecipeCatalogEntry struct {
	RecipeID             int                   `json:"recipeID"`
	QueueTypeID          int                   `json:"queueTypeID"`
	RecipeGroupID        int                   `json:"recipeGroupID"`
	ResearchGroupID      int                   `json:"researchGroupID,omitempty"`
	Level                int                   `json:"level"`
	Type                 string                `json:"type"`
	DurationSec          int                   `json:"durationSec"`
	SkipCostRubies       int                   `json:"skipCostRubies"`
	RequiredBuildingWIDs []int                 `json:"requiredBuildingWIDs,omitempty"`
	Costs                map[string]float64    `json:"costs"`
	Output               AutoSceatRecipeOutput `json:"output"`
}

// AutoSceatRecipeOutput is the item granted by one crafting recipe.
type AutoSceatRecipeOutput struct {
	RewardID int     `json:"rewardID"`
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Amount   float64 `json:"amount"`
	IconURL  string  `json:"iconUrl,omitempty"`
}

// AutoSceatBuildingState describes a live crafting building and its effective unlocks.
type AutoSceatBuildingState struct {
	QueueTypeID        int    `json:"queueTypeID"`
	Name               string `json:"name"`
	OID                int    `json:"oid"`
	WID                int    `json:"wid"`
	ActiveCapacity     int    `json:"activeCapacity"`
	QueueCapacity      int    `json:"queueCapacity"`
	ActiveRecipes      []int  `json:"activeRecipes"`
	QueuedRecipes      []int  `json:"queuedRecipes"`
	AvailableRecipeIDs []int  `json:"availableRecipeIDs"`
}

// AutoSceatMarketState explains the live and calculated capacity of one castle marketplace.
type AutoSceatMarketState struct {
	Loaded                    bool    `json:"loaded"`
	BaseBarrows               int     `json:"baseBarrows"`
	BuildItemBarrows          int     `json:"buildItemBarrows"`
	OtherBarrows              int     `json:"otherBarrows"`
	TotalBarrows              int     `json:"totalBarrows"`
	AvailableBarrows          int     `json:"availableBarrows"`
	BusyBarrows               int     `json:"busyBarrows"`
	CaravanLevel              int     `json:"caravanLevel"`
	CaravanBoostPercent       float64 `json:"caravanBoostPercent"`
	AreaCapacityBoostPercent  float64 `json:"areaCapacityBoostPercent"`
	CapacityBonus             float64 `json:"capacityBonus"`
	CapacityPerBarrow         int     `json:"capacityPerBarrow"`
	AvailableShipmentCapacity int     `json:"availableShipmentCapacity"`
}

// AutoSceatStorageNode is one owned castle which can source, receive, buffer, or craft resources.
type AutoSceatStorageNode struct {
	CastleID    int                      `json:"castleID"`
	Name        string                   `json:"name"`
	Role        string                   `json:"role"`
	KingdomID   int                      `json:"kingdomID"`
	CanCraft    bool                     `json:"canCraft"`
	StormBuffer bool                     `json:"stormBuffer"`
	Resources   map[string]float64       `json:"resources"`
	Storage     map[string]float64       `json:"storage"`
	Market      *AutoSceatMarketState    `json:"market,omitempty"`
	Buildings   []AutoSceatBuildingState `json:"buildings"`
}

// AutoSceatResCatalog is the static official recipe data plus the player's current topology/unlocks.
type AutoSceatResCatalog struct {
	Recipes        []AutoSceatRecipeCatalogEntry    `json:"recipes"`
	Resources      map[string]AutoSceatResourceMeta `json:"resources"`
	Nodes          []AutoSceatStorageNode           `json:"nodes"`
	ResearchLoaded bool                             `json:"researchLoaded"`
}

type autoSceatStaticCatalog struct {
	Recipes                      []AutoSceatRecipeCatalogEntry
	Resources                    map[string]AutoSceatResourceMeta
	MarketBarrowsByWID           map[int]int
	MarketBarrowsByCID           map[int]int
	MarketBoostByLevel           map[int]float64
	MarketCapacityBoostEffectIDs map[int]bool
	MarketCapacityBonusEffectIDs map[int]bool
}

var (
	autoSceatCatalogOnce sync.Once
	autoSceatCatalogData autoSceatStaticCatalog
	autoSceatCatalogErr  error
)

var autoSceatCurrencyAssetHashes = map[string]string{
	"RefinedLumber":        "1623158072971",
	"RefinedStone":         "1623158072971",
	"Component1":           "1623158072971",
	"Component2":           "1623158072971",
	"Component3":           "1623158072971",
	"Component4":           "1623158072971",
	"Component5":           "1623158072971",
	"Component6":           "1623158072971",
	"Component7":           "1623158072971",
	"Component8":           "1623158072971",
	"DragonCharm":          "1716552645290",
	"DragonGlass":          "1716552645290",
	"DragonGlassArrows":    "1716552645290",
	"DragonScaleArmor":     "1716552645290",
	"DragonScaleArrows":    "1716552645290",
	"DragonScaleSplinters": "1716552645290",
	"Steel":                "1716552645290",
	"TwinFlameAxes":        "1716552645290",
	"DragonScaleTile":      "1712658370052",
	"LegendaryToken":       "1573584429307",
	"LegendaryMaterial":    "1573584429307",
	"SceatToken":           "1589213532132",
}

func autoSceatOfficialCurrencyIcon(assetName string) string {
	hash := autoSceatCurrencyAssetHashes[assetName]
	if assetName == "" || hash == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://empire-html5.goodgamestudios.com/default/assets/itemassets/Collectables/Collectable_Currency_%s/Collectable_Currency_%s--%s.webp",
		assetName,
		assetName,
		hash,
	)
}

func autoSceatInt(raw interface{}) int {
	switch value := raw.(type) {
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func autoSceatFloat(raw interface{}) float64 {
	switch value := raw.(type) {
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}

func autoSceatString(raw interface{}) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func autoSceatDecodeRows(data []byte) ([]map[string]interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var rows []map[string]interface{}
	if err := decoder.Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func autoSceatWords(value string) string {
	if value == "" {
		return "Unknown"
	}
	var out []rune
	for index, current := range value {
		if index > 0 && unicode.IsUpper(current) {
			previous := rune(value[index-1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) {
				out = append(out, ' ')
			}
		}
		out = append(out, current)
	}
	return string(out)
}

func autoSceatResourceKey(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	switch strings.ToUpper(suffix) {
	case "C1":
		return "coins"
	case "C2":
		return "rubies"
	case "SCEATTOKEN":
		return "sceatToken"
	}
	if suffix == "" {
		return ""
	}
	return strings.ToLower(suffix[:1]) + suffix[1:]
}

func autoSceatRequiredBuildings(value string) []int {
	var output []int
	for _, part := range strings.Split(value, ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && id > 0 {
			output = append(output, id)
		}
	}
	return output
}

func loadAutoSceatStaticCatalog() (autoSceatStaticCatalog, error) {
	recipeData, err := serverdata.ReadCraftingRecipesJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	rewardData, err := serverdata.ReadRewardsItemsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	currencyData, err := serverdata.ReadCurrenciesItemsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	buildingData, err := serverdata.ReadBuildingsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	constructionItemData, err := serverdata.ReadConstructionItemsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	effectData, err := serverdata.ReadEffectsItemsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	levelBoosterData, err := serverdata.ReadLevelBoostersItemsJSON()
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	recipeRows, err := autoSceatDecodeRows(recipeData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	rewardRows, err := autoSceatDecodeRows(rewardData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	currencyRows, err := autoSceatDecodeRows(currencyData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	buildingRows, err := autoSceatDecodeRows(buildingData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	constructionItemRows, err := autoSceatDecodeRows(constructionItemData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	effectRows, err := autoSceatDecodeRows(effectData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}
	levelBoosterRows, err := autoSceatDecodeRows(levelBoosterData)
	if err != nil {
		return autoSceatStaticCatalog{}, err
	}

	marketBarrowsByWID := make(map[int]int)
	for _, row := range buildingRows {
		barrows := autoSceatInt(row["marketCarriages"])
		wid := autoSceatInt(row["wodID"])
		if barrows > 0 && wid > 0 && autoSceatString(row["name"]) == "Market" {
			marketBarrowsByWID[wid] = barrows
		}
	}
	marketBarrowsByCID := make(map[int]int)
	for _, row := range constructionItemRows {
		barrows := autoSceatInt(row["marketCarriages"])
		cid := autoSceatInt(row["constructionItemID"])
		if barrows > 0 && cid > 0 {
			marketBarrowsByCID[cid] = barrows
		}
	}
	marketBoostByLevel := make(map[int]float64)
	for _, row := range levelBoosterRows {
		if autoSceatInt(row["boosterType"]) != autoSceatCaravanOverloaderBoosterID {
			continue
		}
		marketBoostByLevel[autoSceatInt(row["level"])] = autoSceatFloat(row["boostPercentage"])
	}
	marketCapacityBoostEffectIDs := make(map[int]bool)
	marketCapacityBonusEffectIDs := make(map[int]bool)
	for _, row := range effectRows {
		effectID := autoSceatInt(row["effectID"])
		switch autoSceatString(row["name"]) {
		case "marketCarriageCapacityBoost", "marketCarriageCapacityBoostCapped":
			marketCapacityBoostEffectIDs[effectID] = true
		case "marketCarriageCapacityBonus":
			marketCapacityBonusEffectIDs[effectID] = true
		}
	}

	resources := map[string]AutoSceatResourceMeta{
		"wood":       {Key: "wood", Name: "Wood", JSONKey: "W", IconURL: "/game-data/resources/images/Wood.webp"},
		"stone":      {Key: "stone", Name: "Stone", JSONKey: "S", IconURL: "/game-data/resources/images/Stone.webp"},
		"coal":       {Key: "coal", Name: "Coal", JSONKey: "C", IconURL: "/game-data/resources/images/Charcoal.webp"},
		"oil":        {Key: "oil", Name: "Oil", JSONKey: "O", IconURL: "/game-data/resources/images/OliveOil.webp"},
		"glass":      {Key: "glass", Name: "Glass", JSONKey: "G", IconURL: "/game-data/resources/images/Glass.webp"},
		"iron":       {Key: "iron", Name: "Iron", JSONKey: "I", IconURL: "/game-data/resources/images/Iron_Ore.webp"},
		"coins":      {Key: "coins", Name: "Coins", JSONKey: "C1", IconURL: "/game-data/resources/images/Coins.webp"},
		"rubies":     {Key: "rubies", Name: "Rubies", JSONKey: "C2", IconURL: "/game-data/resources/images/Ruby.webp"},
		"sceatToken": {Key: "sceatToken", Name: "Sceat", JSONKey: "STP", AssetName: "SceatToken", IconURL: "/game-data/resources/images/Sceat.webp"},
	}
	currencyByName := make(map[string]AutoSceatResourceMeta)
	for _, row := range currencyRows {
		name := autoSceatString(row["Name"])
		assetName := autoSceatString(row["assetName"])
		if name == "" {
			continue
		}
		meta := AutoSceatResourceMeta{
			Key:       autoSceatResourceKey(name),
			Name:      autoSceatWords(name),
			JSONKey:   autoSceatString(row["JSONKey"]),
			AssetName: assetName,
			IconURL:   autoSceatOfficialCurrencyIcon(assetName),
		}
		if name == "SceatToken" {
			meta.Name = "Sceat"
			meta.IconURL = "/game-data/resources/images/Sceat.webp"
		}
		currencyByName[name] = meta
		resources[meta.Key] = meta
	}

	rewards := make(map[int]AutoSceatRecipeOutput, len(rewardRows))
	for _, row := range rewardRows {
		rewardID := autoSceatInt(row["rewardID"])
		if rewardID <= 0 {
			continue
		}
		for key, raw := range row {
			if !strings.HasPrefix(key, "add") || autoSceatFloat(raw) <= 0 {
				continue
			}
			suffix := strings.TrimPrefix(key, "add")
			meta, found := currencyByName[suffix]
			if !found {
				resourceKey := autoSceatResourceKey(suffix)
				meta, found = resources[resourceKey]
				if !found {
					meta = AutoSceatResourceMeta{Key: resourceKey, Name: autoSceatWords(suffix)}
					resources[resourceKey] = meta
				}
			}
			name := meta.Name
			if name == "" {
				name = autoSceatString(row["comment2"])
			}
			rewards[rewardID] = AutoSceatRecipeOutput{
				RewardID: rewardID,
				Key:      meta.Key,
				Name:     name,
				Amount:   autoSceatFloat(raw),
				IconURL:  meta.IconURL,
			}
			break
		}
	}

	recipes := make([]AutoSceatRecipeCatalogEntry, 0, len(recipeRows))
	for _, row := range recipeRows {
		recipeID := autoSceatInt(row["craftingRecipeId"])
		queueTypeID := autoSceatInt(row["queueTypeId"])
		if recipeID <= 0 || queueTypeID < 1 || queueTypeID > 4 {
			continue
		}
		entry := AutoSceatRecipeCatalogEntry{
			RecipeID:             recipeID,
			QueueTypeID:          queueTypeID,
			RecipeGroupID:        autoSceatInt(row["recipeGroupID"]),
			ResearchGroupID:      autoSceatInt(row["researchGroupID"]),
			Level:                autoSceatInt(row["level"]),
			Type:                 autoSceatString(row["type"]),
			DurationSec:          autoSceatInt(row["craftingDuration"]),
			SkipCostRubies:       autoSceatInt(row["skipCostC2"]),
			RequiredBuildingWIDs: autoSceatRequiredBuildings(autoSceatString(row["requiredCraftingBuildings"])),
			Costs:                make(map[string]float64),
		}
		for key, raw := range row {
			if !strings.HasPrefix(key, "cost") {
				continue
			}
			amount := autoSceatFloat(raw)
			if amount <= 0 {
				continue
			}
			resourceKey := autoSceatResourceKey(strings.TrimPrefix(key, "cost"))
			entry.Costs[resourceKey] = amount
			if _, found := resources[resourceKey]; !found {
				resources[resourceKey] = AutoSceatResourceMeta{Key: resourceKey, Name: autoSceatWords(strings.TrimPrefix(key, "cost"))}
			}
		}
		entry.Output = rewards[autoSceatInt(row["rewardIDs"])]
		if entry.Output.RewardID == 0 {
			entry.Output = AutoSceatRecipeOutput{RewardID: autoSceatInt(row["rewardIDs"]), Name: "Unknown reward"}
		}
		recipes = append(recipes, entry)
	}
	sort.Slice(recipes, func(i, j int) bool { return recipes[i].RecipeID < recipes[j].RecipeID })
	return autoSceatStaticCatalog{
		Recipes:                      recipes,
		Resources:                    resources,
		MarketBarrowsByWID:           marketBarrowsByWID,
		MarketBarrowsByCID:           marketBarrowsByCID,
		MarketBoostByLevel:           marketBoostByLevel,
		MarketCapacityBoostEffectIDs: marketCapacityBoostEffectIDs,
		MarketCapacityBonusEffectIDs: marketCapacityBonusEffectIDs,
	}, nil
}

func getAutoSceatStaticCatalog() (autoSceatStaticCatalog, error) {
	autoSceatCatalogOnce.Do(func() {
		autoSceatCatalogData, autoSceatCatalogErr = loadAutoSceatStaticCatalog()
	})
	return autoSceatCatalogData, autoSceatCatalogErr
}

func autoSceatQueueName(queueID int) string {
	switch queueID {
	case 1:
		return "Refinery"
	case 2:
		return "Toolsmith"
	case 3:
		return "Dragon Hoard"
	case 4:
		return "Dragon Forge"
	default:
		return fmt.Sprintf("Crafting Queue %d", queueID)
	}
}

func autoSceatContainsInt(items []int, value int) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func autoSceatRecipeAvailable(recipe AutoSceatRecipeCatalogEntry, building castle.CraftingBuildingSnapshot, entitlements castle.CraftingEntitlements) bool {
	if recipe.QueueTypeID != building.CQID {
		return false
	}
	if len(recipe.RequiredBuildingWIDs) > 0 && !autoSceatContainsInt(recipe.RequiredBuildingWIDs, building.WID) {
		return false
	}
	if recipe.ResearchGroupID == 0 {
		return true
	}
	return autoSceatContainsInt(entitlements.EnabledRecipeIDs, recipe.RecipeID) ||
		autoSceatContainsInt(entitlements.EnabledRecipeGroupIDs, recipe.RecipeGroupID)
}

func autoSceatBuildingStates(info *Models.PlayerCastleInfo, recipes []AutoSceatRecipeCatalogEntry) []AutoSceatBuildingState {
	if info == nil {
		return nil
	}
	states := make([]AutoSceatBuildingState, 0, len(info.CraftingQueues))
	for _, building := range info.CraftingQueues {
		state := AutoSceatBuildingState{
			QueueTypeID:        building.CQID,
			Name:               autoSceatQueueName(building.CQID),
			OID:                building.OID,
			WID:                building.WID,
			ActiveCapacity:     1 + len(building.PS.RUT),
			QueueCapacity:      1 + len(building.QS.RUT),
			ActiveRecipes:      append([]int{}, building.PS.CRID...),
			QueuedRecipes:      append([]int{}, building.QS.CRID...),
			AvailableRecipeIDs: make([]int, 0),
		}
		for _, recipe := range recipes {
			if autoSceatRecipeAvailable(recipe, building, info.CraftingEntitlements) {
				state.AvailableRecipeIDs = append(state.AvailableRecipeIDs, recipe.RecipeID)
			}
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].QueueTypeID < states[j].QueueTypeID })
	return states
}

func autoSceatNodeResources(info *Models.PlayerCastleInfo) (map[string]float64, map[string]float64) {
	if info == nil {
		return map[string]float64{}, map[string]float64{}
	}
	amount := info.Amount
	storage := info.Storage
	return map[string]float64{
			"wood": amount.WoodAmount, "stone": amount.StoneAmount, "coal": amount.CoalAmount,
			"oil": amount.OilAmount, "glass": amount.GlassAmount, "iron": amount.IronAmount,
		}, map[string]float64{
			"wood": storage.WoodMax, "stone": storage.StoneMax, "coal": storage.CoalMax,
			"oil": storage.OilMax, "glass": storage.GlassMax, "iron": storage.IronMax,
		}
}

func autoSceatMarketState(info *Models.PlayerCastleInfo, static autoSceatStaticCatalog, snapshot gamestate.MarketTransportState) *AutoSceatMarketState {
	if info == nil || info.Aid <= 0 {
		return nil
	}
	marketOID := 0
	baseBarrows := 0
	for _, building := range info.AllBuildingRows() {
		barrows := static.MarketBarrowsByWID[building.BuildingID]
		if barrows > baseBarrows {
			baseBarrows = barrows
			marketOID = building.OID
		}
	}
	buildItemBarrows := 0
	if marketOID > 0 {
		for _, building := range info.ConstructionByBuilding {
			if building.OID != marketOID {
				continue
			}
			for _, slot := range building.Slots {
				buildItemBarrows += static.MarketBarrowsByCID[slot.CID]
			}
		}
	}

	marketCastle := gamestate.MarketCastleState{}
	marketLoaded := false
	for _, candidate := range snapshot.Castles {
		if candidate.CastleID == int(info.Aid) {
			marketCastle = candidate
			marketLoaded = true
			break
		}
	}
	totalBarrows := marketCastle.TotalBarrows
	availableBarrows := marketCastle.AvailableBarrows
	if !marketLoaded {
		totalBarrows = info.MarketBarrowsTotal
		availableBarrows = info.MarketBarrowsAvailable
	}
	if totalBarrows <= 0 {
		totalBarrows = baseBarrows + buildItemBarrows
	}
	if totalBarrows <= 0 && baseBarrows <= 0 {
		return nil
	}
	if availableBarrows < 0 {
		availableBarrows = 0
	}
	if availableBarrows > totalBarrows {
		availableBarrows = totalBarrows
	}

	areaBoost := float64(0)
	capacityBonus := float64(0)
	for _, effect := range marketCastle.AreaEffects {
		if len(effect.Values) == 0 {
			continue
		}
		if static.MarketCapacityBoostEffectIDs[effect.EffectID] {
			areaBoost += effect.Values[0]
		}
		if static.MarketCapacityBonusEffectIDs[effect.EffectID] {
			capacityBonus += effect.Values[0]
		}
	}
	caravanBoost := static.MarketBoostByLevel[snapshot.CaravanLevel]
	capacity := float64(autoSceatBaseCapacityPerBarrow) * (1 + areaBoost/100) * (1 + caravanBoost/100)
	capacity += capacityBonus
	capacity = math.Round(capacity*10) / 10
	capacityPerBarrow := 1 + int(math.Round(capacity))
	otherBarrows := totalBarrows - baseBarrows - buildItemBarrows
	if otherBarrows < 0 {
		otherBarrows = 0
	}
	return &AutoSceatMarketState{
		Loaded:                    marketLoaded && snapshot.CaravanLevelLoaded,
		BaseBarrows:               baseBarrows,
		BuildItemBarrows:          buildItemBarrows,
		OtherBarrows:              otherBarrows,
		TotalBarrows:              totalBarrows,
		AvailableBarrows:          availableBarrows,
		BusyBarrows:               totalBarrows - availableBarrows,
		CaravanLevel:              snapshot.CaravanLevel,
		CaravanBoostPercent:       caravanBoost,
		AreaCapacityBoostPercent:  areaBoost,
		CapacityBonus:             capacityBonus,
		CapacityPerBarrow:         capacityPerBarrow,
		AvailableShipmentCapacity: availableBarrows * capacityPerBarrow,
	}
}

func autoSceatNode(info *Models.PlayerCastleInfo, role string, fallbackKingdomID int, canCraft, storm bool, static autoSceatStaticCatalog, market gamestate.MarketTransportState) (AutoSceatStorageNode, bool) {
	if info == nil || info.Aid <= 0 {
		return AutoSceatStorageNode{}, false
	}
	kingdomID := info.MapKingdomID
	if kingdomID == 0 && fallbackKingdomID != 0 {
		kingdomID = fallbackKingdomID
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		name = role
	}
	resources, storage := autoSceatNodeResources(info)
	return AutoSceatStorageNode{
		CastleID:    int(info.Aid),
		Name:        name,
		Role:        role,
		KingdomID:   kingdomID,
		CanCraft:    canCraft,
		StormBuffer: storm,
		Resources:   resources,
		Storage:     storage,
		Market:      autoSceatMarketState(info, static, market),
		Buildings:   autoSceatBuildingStates(info, static.Recipes),
	}, true
}

// BuildAutoSceatResCatalog builds the UI catalog from official data and the current player state.
func BuildAutoSceatResCatalog() (AutoSceatResCatalog, error) {
	static, err := getAutoSceatStaticCatalog()
	if err != nil {
		return AutoSceatResCatalog{}, err
	}

	result := AutoSceatResCatalog{
		Recipes:   static.Recipes,
		Resources: static.Resources,
	}
	gs := Models.GetGameState()
	market := gs.MarketTransportSnapshot()
	c := &gs.Castle
	definitions := []struct {
		info    *Models.PlayerCastleInfo
		role    string
		kingdom int
		craft   bool
		storm   bool
	}{
		{&c.MainCastle, "Main Castle", 0, true, false},
		{&c.Outpost1, "Outpost 1", 0, false, false},
		{&c.Outpost2, "Outpost 2", 0, false, false},
		{&c.Outpost3, "Outpost 3", 0, false, false},
		{&c.Metropolis, "Metropolis", 0, false, false},
		{&c.Capital, "Capital", 0, false, false},
		{&c.DesertCastle, "Burning Sands", 1, true, false},
		{&c.IceCastle, "Everwinter Glacier", 2, true, false},
		{&c.DungeonCastle, "Fire Peaks", 3, true, false},
		{&c.StormCastle, "Storm Islands", 4, false, true},
	}
	for _, definition := range definitions {
		node, ok := autoSceatNode(definition.info, definition.role, definition.kingdom, definition.craft, definition.storm, static, market)
		if !ok {
			continue
		}
		for _, building := range node.Buildings {
			if len(building.AvailableRecipeIDs) > 0 {
				result.ResearchLoaded = true
			}
		}
		result.Nodes = append(result.Nodes, node)
	}
	return result, nil
}

// AutoSceatRecipeByID returns a static official recipe for automation decisions.
func AutoSceatRecipeByID(recipeID int) (AutoSceatRecipeCatalogEntry, bool) {
	static, err := getAutoSceatStaticCatalog()
	if err != nil {
		return AutoSceatRecipeCatalogEntry{}, false
	}
	index := sort.Search(len(static.Recipes), func(i int) bool { return static.Recipes[i].RecipeID >= recipeID })
	if index >= len(static.Recipes) || static.Recipes[index].RecipeID != recipeID {
		return AutoSceatRecipeCatalogEntry{}, false
	}
	return static.Recipes[index], true
}
