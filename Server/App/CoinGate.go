package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const coinResourceKey = "C1"

// coinDispatchGate is process-local: pending debits deliberately do not
// survive a restart, while live C1 authority also does not survive a restart.
// A restarted runtime therefore cannot spend until a new current-session C1
// snapshot arrives.
type coinDispatchGate struct {
	mu               sync.Mutex
	pending          map[string]pendingCoinDebit
	watermark        State.PlayerResourceObservation
	watermarkBalance int64
}

type pendingCoinDebit struct {
	amount               int64
	reserve              int64
	observedBalance      int64
	observedAt           time.Time
	connectionGeneration uint64
	reservedAt           time.Time
	completedAt          time.Time
	indeterminateAt      time.Time
}

type resolvedCoinCost struct {
	amount     int64
	reserve    int64
	source     string
	upperBound bool
	additive   bool
}

func newCoinDispatchGate() *coinDispatchGate {
	return &coinDispatchGate{pending: map[string]pendingCoinDebit{}}
}

func (gate *coinDispatchGate) Validate(
	ctx context.Context,
	input Intent.PlanningContext,
	step Intent.Step,
) error {
	cost, covered, err := resolveCoinCost(input, step)
	if err != nil {
		return err
	}
	if !covered || cost.amount <= 0 {
		return nil
	}
	resourceID, err := officialResourceIDByJSONKey(input.GameData, coinResourceKey)
	if err != nil {
		return Localization.WithError(fmt.Errorf("coin affordability unavailable: %w", err), Localization.ErrorContext(Localization.New("server.app.coin_affordability_unavailable.a3c889be", "coin affordability unavailable", nil), err))
	}
	observation, observed := input.State.Player.ResourceObservations[State.ResourceID(resourceID)]
	if input.State.Session.ConnectionGeneration == 0 || !observed || observation.ObservedAt.IsZero() ||
		observation.ConnectionGeneration != input.State.Session.ConnectionGeneration {
		return Localization.WithError(fmt.Errorf("coin affordability unavailable: current-session authoritative C1 balance is missing"), Localization.New("server.app.coin_affordability_unavailable_current.3afa2f9b", "coin affordability unavailable: current-session authoritative C1 balance is missing", nil))
	}
	coinBalance := input.State.Player.Resources[State.ResourceID(resourceID)]
	if coinBalance < 0 || math.IsNaN(coinBalance) || math.IsInf(coinBalance, 0) || coinBalance >= math.Exp2(63) {
		return Localization.WithError(fmt.Errorf("coin affordability unavailable: authoritative C1 balance is malformed"), Localization.New("server.app.coin_affordability_unavailable_authoritative.a853c8f4", "coin affordability unavailable: authoritative C1 balance is malformed", nil))
	}
	coins := int64(math.Floor(coinBalance))
	now := time.Now().UTC()
	key := coinDispatchKey(ctx, step)
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.watermark.ConnectionGeneration > observation.ConnectionGeneration ||
		(gate.watermark.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Before(gate.watermark.ObservedAt)) ||
		(gate.watermark.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Equal(gate.watermark.ObservedAt) &&
			!gate.watermark.ObservedAt.IsZero() && coins != gate.watermarkBalance) {
		return Localization.WithError(fmt.Errorf("%w: coin balance snapshot predates the shared affordability gate", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.e2fabc80", "intent plan became stale before dispatch: coin balance snapshot predates the shared affordability gate", nil))
	}
	if observation.ConnectionGeneration > gate.watermark.ConnectionGeneration || observation.ObservedAt.After(gate.watermark.ObservedAt) {
		gate.watermark = observation
		gate.watermarkBalance = coins
	}
	gate.reconcile(observation, coins)
	existing, exists := gate.pending[key]
	if exists {
		if existing.amount != cost.amount {
			return Localization.WithError(fmt.Errorf("coin affordability unavailable: repeated dispatch changed cost from %d to %d", existing.amount, cost.amount), Localization.New("server.app.coin_affordability_unavailable_repeated.0b255132", "coin affordability unavailable: repeated dispatch changed cost from {p0} to {p1}", Localization.Params{"p0": existing.amount, "p1": cost.amount}))
		}
		if existing.reserve != cost.reserve {
			return Localization.WithError(fmt.Errorf("coin affordability unavailable: repeated dispatch changed reserve from %d to %d", existing.reserve, cost.reserve), Localization.New("server.app.coin_affordability_unavailable_repeated.bded7a41", "coin affordability unavailable: repeated dispatch changed reserve from {p0} to {p1}", Localization.Params{"p0": existing.reserve, "p1": cost.reserve}))
		}
	}
	pending := int64(0)
	for candidate, debit := range gate.pending {
		if candidate != key {
			if debit.amount < 0 || pending > math.MaxInt64-debit.amount {
				return Localization.WithError(fmt.Errorf("coin affordability unavailable: pending debit overflowed"), Localization.New("server.app.coin_affordability_unavailable_pending.c698b690", "coin affordability unavailable: pending debit overflowed", nil))
			}
			pending += debit.amount
		}
	}
	if cost.amount > math.MaxInt64-cost.reserve || pending > math.MaxInt64-cost.amount-cost.reserve ||
		coins < pending+cost.amount+cost.reserve {
		return &Intent.CoinUnavailableError{
			Required: cost.amount, Reserve: cost.reserve, Observed: coins, Pending: pending, Source: cost.source,
		}
	}
	if exists {
		return nil
	}
	// Router layers may invoke final validation more than once. Reusing the
	// operation/response key makes the reservation idempotent.
	gate.pending[key] = pendingCoinDebit{
		amount: cost.amount, reserve: cost.reserve, observedBalance: coins, observedAt: observation.ObservedAt,
		connectionGeneration: observation.ConnectionGeneration, reservedAt: now,
	}
	return nil
}

func (gate *coinDispatchGate) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	key := coinDispatchKey(ctx, step)
	delete(gate.pending, key)
	gate.mu.Unlock()
}

func (gate *coinDispatchGate) Indeterminate(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	key := coinDispatchKey(ctx, step)
	if debit, exists := gate.pending[key]; exists {
		debit.indeterminateAt = time.Now().UTC()
		gate.pending[key] = debit
	}
	gate.mu.Unlock()
}

func (gate *coinDispatchGate) Completed(
	ctx context.Context,
	input Intent.PlanningContext,
	step Intent.Step,
	response Protocol.CommittedFrame,
) bool {
	gate.mu.Lock()
	key := coinDispatchKey(ctx, step)
	debit, exists := gate.pending[key]
	if !exists {
		gate.mu.Unlock()
		return false
	}
	debit.completedAt = time.Now().UTC()
	gate.pending[key] = debit
	resourceID, err := officialResourceIDByJSONKey(input.GameData, coinResourceKey)
	observation, observed := input.State.Player.ResourceObservations[State.ResourceID(resourceID)]
	// The exact response timestamp is the correlation proof. PlanningContext
	// can contain unrelated income or refreshes committed while this command was
	// queued/in flight, so merely seeing a newer observation is insufficient.
	correlated := err == nil && response.ReduceError == "" && !response.Frame.ReceivedAt.IsZero() && observed &&
		observation.ConnectionGeneration == debit.connectionGeneration &&
		observation.ObservedAt.Equal(response.Frame.ReceivedAt)
	if correlated {
		balance := input.State.Player.Resources[State.ResourceID(resourceID)]
		if balance >= 0 && !math.IsNaN(balance) && !math.IsInf(balance, 0) && balance < math.Exp2(63) &&
			(observation.ConnectionGeneration > gate.watermark.ConnectionGeneration || !observation.ObservedAt.Before(gate.watermark.ObservedAt)) {
			gate.watermark = observation
			gate.watermarkBalance = int64(math.Floor(balance))
		}
		delete(gate.pending, key)
	}
	gate.mu.Unlock()
	return !correlated
}

func (gate *coinDispatchGate) reconcile(observation State.PlayerResourceObservation, balance int64) {
	for key, debit := range gate.pending {
		if observation.ConnectionGeneration != debit.connectionGeneration {
			if observation.ConnectionGeneration > 0 && observation.ObservedAt.After(debit.reservedAt) {
				delete(gate.pending, key)
			}
			continue
		}
		// Successful commands wait for a snapshot ordered after completion.
		// Uncertain commands wait for a new snapshot ordered after the uncertain
		// outcome. A balance refresh between reservation and acknowledgement can
		// never release either debit.
		reconcileAfter := debit.completedAt
		if reconcileAfter.IsZero() {
			reconcileAfter = debit.indeterminateAt
		}
		if !reconcileAfter.IsZero() && observation.ObservedAt.After(reconcileAfter) {
			_ = balance // the authoritative post-dispatch snapshot is the new budget
			delete(gate.pending, key)
		}
	}
}

func coinDispatchKey(ctx context.Context, step Intent.Step) string {
	metadata := Outbound.MetadataFromContext(ctx)
	if token := strings.TrimSpace(metadata.ResponseToken); token != "" {
		return token
	}
	hash := sha256.Sum256(append([]byte(strings.ToLower(step.Opcode)+"\x00"), step.Payload...))
	return strings.TrimSpace(metadata.OperationID) + "/" + hex.EncodeToString(hash[:8])
}

func resolveCoinCost(input Intent.PlanningContext, step Intent.Step) (resolvedCoinCost, bool, error) {
	declared := resolvedCoinCost{}
	if step.CoinCost != nil {
		if step.CoinCost.Amount < 0 || step.CoinCost.Reserve < 0 || strings.TrimSpace(step.CoinCost.Source) == "" {
			return resolvedCoinCost{}, true, Localization.WithError(fmt.Errorf("coin affordability unavailable: declared cost is malformed"), Localization.New("server.app.coin_affordability_unavailable_declared.6eb67ba9", "coin affordability unavailable: declared cost is malformed", nil))
		}
		declared = resolvedCoinCost{
			amount: step.CoinCost.Amount, reserve: step.CoinCost.Reserve,
			source: strings.TrimSpace(step.CoinCost.Source), upperBound: step.CoinCost.UpperBound,
			additive: step.CoinCost.Additive,
		}
	}
	opcode := strings.ToLower(strings.TrimSpace(step.Opcode))
	var computed resolvedCoinCost
	var covered bool
	var err error
	switch opcode {
	case "bup":
		computed, err = productionCoinCost(input.GameData, step.Payload)
		covered = true
	case "hru":
		computed, err = hospitalCoinCost(input.GameData, step.Payload)
		covered = true
	case "cds":
		computed, err = supportCoinCost(input, step.Payload)
		covered = true
	case "cra":
		computed, err = attackCoinCost(input, step.Payload)
		covered = true
	case "csm":
		computed, err = spyCoinCost(input, step.Payload)
		covered = true
	case "crm":
		computed, err = marketShipmentCoinCost(input, step.Payload)
		covered = true
	case "ere":
		computed, err = relicEnchantCoinCost(input, step.Payload)
		covered = true
	case "eqe":
		computed, err = equipmentEnchantCoinCost(input, step.Payload)
		covered = true
	case "bge":
		computed, err = gemInsertionCoinCost(input, step.Payload)
		covered = true
	case "kut":
		computed, err = kingdomTroopTransferCoinCost(input, step.Payload)
		covered = true
	case "sbp":
		computed, err = shopPackageCoinCost(input.GameData, step.Payload)
		covered = true
	}
	if err != nil {
		return resolvedCoinCost{}, true, fmt.Errorf("coin affordability unavailable for %s: %w", strings.ToUpper(opcode), err)
	}
	if declared.additive {
		if declared.amount > math.MaxInt64-computed.amount {
			return resolvedCoinCost{}, true, Localization.WithError(fmt.Errorf("coin affordability unavailable: combined cost overflowed"), Localization.New("server.app.coin_affordability_unavailable_combined.31db260b", "coin affordability unavailable: combined cost overflowed", nil))
		}
		computed.amount += declared.amount
	} else if declared.amount > computed.amount {
		computed.amount = declared.amount
	}
	if declared.reserve > computed.reserve {
		computed.reserve = declared.reserve
	}
	if declared.source != "" {
		if computed.source == "" {
			computed.source = declared.source
		} else {
			computed.source += "; " + declared.source
		}
		computed.upperBound = computed.upperBound || declared.upperBound
		covered = true
	}
	return computed, covered, nil
}

func shopPackageCoinCost(store *GameData.Store, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		PackageID int64 `json:"PID"`
		Amount    int64 `json:"AMT"`
		BuyAll    int64 `json:"BA"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.PackageID <= 0 || request.Amount <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved shop package purchase is malformed"), Localization.New("server.app.resolved_shop_package_purchase.a22dbce5", "resolved shop package purchase is malformed", nil))
	}
	unitCost, known, err := officialNumberOrZero(store, "packages", request.PackageID, "packagePriceC1")
	if err != nil || !known || unitCost < 0 || math.IsNaN(unitCost) || math.IsInf(unitCost, 0) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official package %d coin price is missing or malformed", request.PackageID), Localization.New("server.app.official_package_p_coin.0aed854e", "official package {p0} coin price is missing or malformed", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID)}))
	}
	if request.BuyAll != 0 && unitCost > 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("coin affordability unavailable: shop package buy-all pricing is not authoritative"), Localization.New("server.app.coin_affordability_unavailable_shop.318c76a1", "coin affordability unavailable: shop package buy-all pricing is not authoritative", nil))
	}
	amount, err := checkedCeilProduct(unitCost, request.Amount)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	return resolvedCoinCost{amount: amount, source: "official shop package coin price"}, nil
}

func kingdomTroopTransferCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		TargetKingdom State.KingdomID `json:"TKID"`
		Items         [][2]int64      `json:"A"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.TargetKingdom < 0 || len(request.Items) == 0 || input.GameData == nil {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved kingdom troop transfer is malformed"), Localization.New("server.app.resolved_kingdom_troop_transfer.1dfe18f8", "resolved kingdom troop transfer is malformed", nil))
	}
	tax, known := officialNumber(input.GameData, "kingdoms", int64(request.TargetKingdom), "unitTravelTaxRate")
	if !known || tax < 0 || math.IsNaN(tax) || math.IsInf(tax, 0) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official kingdom %d unit travel tax is unavailable", request.TargetKingdom), Localization.New("server.app.official_kingdom_p_unit.bf259222", "official kingdom {p0} unit travel tax is unavailable", Localization.Params{"p0": request.TargetKingdom}))
	}
	catalog, err := input.GameData.Catalog("units")
	if err != nil {
		return resolvedCoinCost{}, err
	}
	total := float64(0)
	for _, pair := range request.Items {
		if pair[0] <= 0 || pair[1] <= 0 {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("kingdom troop transfer manifest is malformed"), Localization.New("server.app.kingdom_troop_transfer_manifest.aeb8bdc2", "kingdom troop transfer manifest is malformed", nil))
		}
		raw, found := catalog.Find(strconv.FormatInt(pair[0], 10))
		if !found {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official transfer item %d is unavailable", pair[0]), Localization.New("server.app.official_transfer_item_p.2ed0e2da", "official transfer item {p0} is unavailable", Localization.Params{"p0": pair[0]}))
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return resolvedCoinCost{}, err
		}
		unitCost := float64(100)
		if !GameData.IsToolRecord(record) {
			unitCost, _, err = officialNumberOrZero(input.GameData, "units", pair[0], "kingdomTravellingCost")
			if err != nil {
				return resolvedCoinCost{}, err
			}
		}
		unitCost *= tax / 100
		itemCost := unitCost * float64(pair[1])
		if itemCost < 0 || math.IsNaN(itemCost) || math.IsInf(itemCost, 0) || total >= math.Exp2(63)-itemCost {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("kingdom troop transfer coin cost overflowed"), Localization.New("server.app.kingdom_troop_transfer_coin.e35e81ec", "kingdom troop transfer coin cost overflowed", nil))
		}
		total += itemCost
	}
	return resolvedCoinCost{amount: int64(math.Round(total)), source: "official kingdom troop and tool transfer formula", upperBound: true}, nil
}

func equipmentEnchantCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		ItemID   int64 `json:"EID"`
		CostMode int   `json:"C2"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.ItemID <= 0 || request.CostMode != 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved equipment enchantment is malformed"), Localization.New("server.app.resolved_equipment_enchantment_is.e1533dd2", "resolved equipment enchantment is malformed", nil))
	}
	item, found := input.State.Inventory.Equipment[State.EquipmentInstanceID(request.ItemID)]
	if !found || item.Level < 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("equipment %d current enchantment level is unavailable", request.ItemID), Localization.New("server.app.equipment_p_current_enchantment.0e51f1ad", "equipment {p0} current enchantment level is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
	}
	value := 10 * math.Round(17*math.Pow(float64(item.Level+1), 1.7))
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value >= math.Exp2(63) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("equipment enchantment coin cost overflowed"), Localization.New("server.app.equipment_enchantment_coin_cost.d508299f", "equipment enchantment coin cost overflowed", nil))
	}
	return resolvedCoinCost{amount: int64(value), source: "official base equipment enchantment formula", upperBound: true}, nil
}

func relicEnchantCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		ItemID    int64 `json:"RIID"`
		Equipment int   `json:"EQ"`
		CostMode  int   `json:"C2"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.ItemID <= 0 || request.CostMode != 0 || request.Equipment < 0 || request.Equipment > 1 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved relic enchantment is malformed"), Localization.New("server.app.resolved_relic_enchantment_is.86e31d89", "resolved relic enchantment is malformed", nil))
	}
	level := 0
	if request.Equipment == 1 {
		item, found := input.State.Inventory.Equipment[State.EquipmentInstanceID(request.ItemID)]
		if !found {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("relic equipment %d is unavailable", request.ItemID), Localization.New("server.app.relic_equipment_p_is.71ba6506", "relic equipment {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
		}
		level = item.Level
	} else {
		gem, found := input.State.Inventory.Gems[State.GemInstanceID(request.ItemID)]
		if !found {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("relic gem %d is unavailable", request.ItemID), Localization.New("server.app.relic_gem_p_is.8e00b8cf", "relic gem {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
		}
		level = gem.Level
	}
	catalog, err := input.GameData.Catalog("relicEnchanters")
	if err != nil {
		return resolvedCoinCost{}, err
	}
	cost, known := catalog.Float64ByField("level", strconv.Itoa(level+1), "c1Cost")
	if !known || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) || cost >= math.Exp2(63) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official relic enchantment level %d coin cost is unavailable", level+1), Localization.New("server.app.official_relic_enchantment_level.cada5633", "official relic enchantment level {p0} coin cost is unavailable", Localization.Params{"p0": level + 1}))
	}
	return resolvedCoinCost{amount: int64(math.Ceil(cost)), source: "official relic enchantment level cost"}, nil
}

func gemInsertionCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		GemID    int64 `json:"GID"`
		RelicGem int   `json:"RGEM"`
		Mode     int   `json:"M"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.GemID <= 0 || request.Mode != 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved gem insertion is malformed"), Localization.New("server.app.resolved_gem_insertion_is.950f6bd2", "resolved gem insertion is malformed", nil))
	}
	level := int64(-1)
	if request.RelicGem == 1 {
		if _, found := input.State.Inventory.Gems[State.GemInstanceID(request.GemID)]; !found {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("relic gem %d is unavailable", request.GemID), Localization.New("server.app.relic_gem_p_is.8e00b8cf", "relic gem {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.GemID)}))
		}
		return resolvedCoinCost{amount: 95_000, source: "official relic gem insertion constant"}, nil
	} else if request.RelicGem == 0 {
		levelValue, known := officialNumber(input.GameData, "gems", request.GemID, "gemLevelID")
		if !known || levelValue < 0 || levelValue != math.Trunc(levelValue) {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official gem %d level is unavailable", request.GemID), Localization.New("server.app.official_gem_p_level.6b6645cf", "official gem {p0} level is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.GemID)}))
		}
		level = int64(levelValue)
	} else {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved gem insertion kind is malformed"), Localization.New("server.app.resolved_gem_insertion_kind.f92d1f5c", "resolved gem insertion kind is malformed", nil))
	}
	cost, known := officialNumber(input.GameData, "gemlevels", level, "insertCostC1")
	if !known || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) || cost >= math.Exp2(63) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official gem level %d insertion cost is unavailable", level), Localization.New("server.app.official_gem_level_p.5af19e33", "official gem level {p0} insertion cost is unavailable", Localization.Params{"p0": level}))
	}
	return resolvedCoinCost{amount: int64(math.Ceil(cost)), source: "official base gem insertion level cost", upperBound: true}, nil
}

func marketShipmentCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		SourceCastleID State.CastleID `json:"SID"`
		TargetX        int            `json:"TX"`
		TargetY        int            `json:"TY"`
		HorseID        int64          `json:"HBW"`
		Goods          [][]any        `json:"G"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.SourceCastleID <= 0 || len(request.Goods) != 1 || len(request.Goods[0]) < 2 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved market shipment is malformed"), Localization.New("server.app.resolved_market_shipment_is.ca68f454", "resolved market shipment is malformed", nil))
	}
	amount, ok := request.Goods[0][1].(float64)
	if !ok || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || amount >= math.Exp2(63) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved market shipment amount is malformed"), Localization.New("server.app.resolved_market_shipment_amount.4cb17ef5", "resolved market shipment amount is malformed", nil))
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found || input.GameData == nil || !input.State.Market.CaravanLevelLoaded || input.State.Market.ObservedAt.IsZero() {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("current official market capacity is unavailable"), Localization.New("server.app.current_official_market_capacity.2683f292", "current official market capacity is unavailable", nil))
	}
	market, found := input.State.Market.Castles[source.ID]
	if !found {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("source market state is unavailable"), Localization.New("server.app.source_market_state_is.2cac4cbc", "source market state is unavailable", nil))
	}
	effects := make([]GameData.MarketEffect, 0, len(market.AreaEffects))
	for _, effect := range market.AreaEffects {
		effects = append(effects, GameData.MarketEffect{EffectID: effect.EffectID, Values: effect.Values})
	}
	capacity, err := input.GameData.MarketCapacity(input.State.Market.CaravanLevel, effects)
	if err != nil || capacity.CapacityPerBarrow <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("current official market capacity is unavailable"), Localization.New("server.app.current_official_market_capacity.2683f292", "current official market capacity is unavailable", nil))
	}
	barrows := int64(math.Ceil(amount / float64(capacity.CapacityPerBarrow)))
	if barrows <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("market shipment barrow count is malformed"), Localization.New("server.app.market_shipment_barrow_count.7f13f107", "market shipment barrow count is malformed", nil))
	}
	distance := math.Hypot(float64(request.TargetX-source.X), float64(request.TargetY-source.Y))
	logDistance := math.Log(distance+1) / math.Log(2.3)
	baseValue := math.Ceil(3 * float64(barrows) * logDistance)
	if math.IsNaN(baseValue) || math.IsInf(baseValue, 0) || baseValue >= math.Exp2(63) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("market shipment coin cost overflowed"), Localization.New("server.app.market_shipment_coin_cost.e8b41015", "market shipment coin cost overflowed", nil))
	}
	horseItems, err := checkedMultiply(barrows, 5, "market horse item count")
	if err != nil {
		return resolvedCoinCost{}, err
	}
	horse, err := horseCoinCost(input.GameData, request.HorseID, distance, horseItems)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	base := int64(baseValue)
	if horse > math.MaxInt64-base {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("market shipment coin cost overflowed"), Localization.New("server.app.market_shipment_coin_cost.e8b41015", "market shipment coin cost overflowed", nil))
	}
	return resolvedCoinCost{amount: base + horse, source: "official market and horse travel formula"}, nil
}

func checkedMultiply(left, right int64, label string) (int64, error) {
	if left < 0 || right < 0 || left > 0 && right > math.MaxInt64/left {
		return 0, Localization.WithError(fmt.Errorf("%s overflowed", label), Localization.New("server.app.p_overflowed.fefe46a5", "{p0} overflowed", Localization.Params{"p0": fmt.Sprintf("%s", label)}))
	}
	return left * right, nil
}

func spyCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		SourceCastleID State.CastleID `json:"SID"`
		TargetX        int            `json:"TX"`
		TargetY        int            `json:"TY"`
		SpyCount       int64          `json:"SC"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.SourceCastleID <= 0 || request.SpyCount <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved spy command is malformed"), Localization.New("server.app.resolved_spy_command_is.c5139b55", "resolved spy command is malformed", nil))
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("spy source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.spy_source_castle_p.c95b519d", "spy source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	distance := math.Hypot(float64(request.TargetX-source.X), float64(request.TargetY-source.Y))
	// The live payload does not expose the dialog risk term. The official
	// formula caps it at 95, so this is a conservative dispatch bound.
	value := .6 * (20*float64(request.SpyCount)*math.Log(distance+1)/math.Log(2.3) + 95 - 20)
	if value < 0 {
		value = 0
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value >= math.Exp2(31) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("spy travel cost overflowed"), Localization.New("server.app.spy_travel_cost_overflowed.dce03e9d", "spy travel cost overflowed", nil))
	}
	return resolvedCoinCost{amount: int64(int32(value)), source: "official maximum-risk spy travel formula", upperBound: true}, nil
}

func productionCoinCost(store *GameData.Store, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		LineID       int   `json:"LID"`
		DefinitionID int64 `json:"WID"`
		Amount       int64 `json:"AMT"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.DefinitionID <= 0 || request.Amount <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved production command is malformed"), Localization.New("server.app.resolved_production_command_is.fdfbb1b8", "resolved production command is malformed", nil))
	}
	collection := "units"
	if request.LineID == 1 {
		collection = "tools"
	} else if request.LineID != 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("production line %d has no supported coin formula", request.LineID), Localization.New("server.app.production_line_p_has.58b28c76", "production line {p0} has no supported coin formula", Localization.Params{"p0": fmt.Sprintf("%d", request.LineID)}))
	}
	catalogCollection := collection
	if collection == "tools" {
		catalogCollection = "units"
	}
	unitCost, known, err := officialNumberOrZero(store, catalogCollection, request.DefinitionID, "costC1")
	if err != nil || !known || unitCost < 0 || math.IsNaN(unitCost) || math.IsInf(unitCost, 0) {
		return resolvedCoinCost{}, fmt.Errorf("official %s %d costC1 is missing or malformed", strings.TrimSuffix(collection, "s"), request.DefinitionID)
	}
	amount, err := checkedCeilProduct(unitCost, request.Amount)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	return resolvedCoinCost{amount: amount, source: "official base production cost", upperBound: true}, nil
}

// The official production client treats an absent costC1 field on an existing
// valid item as zero. Other cost fields can fund that production. A missing
// record or a present malformed cost remains unavailable rather than free.
func officialNumberOrZero(store *GameData.Store, collection string, id int64, field string) (float64, bool, error) {
	if store == nil {
		return 0, false, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return 0, false, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return 0, false, Localization.WithError(fmt.Errorf("official item %d is unavailable", id), Localization.New("server.app.official_item_p_is.df6d7f77", "official item {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false, err
	}
	if _, exists := record[field]; !exists {
		return 0, true, nil
	}
	value, valid := record.Float64(field)
	if !valid {
		return 0, false, Localization.WithError(fmt.Errorf("official item %d %s is malformed", id, field), Localization.New("server.app.official_item_p_p.ba61ce01", "official item {p0} {p1} is malformed", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%s", field)}))
	}
	return value, true, nil
}

func hospitalCoinCost(store *GameData.Store, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		UnitID int64 `json:"U"`
		Amount int64 `json:"A"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.UnitID <= 0 || request.Amount <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved hospital command is malformed"), Localization.New("server.app.resolved_hospital_command_is.0c4240a0", "resolved hospital command is malformed", nil))
	}
	unitCost, known := officialNumber(store, "units", request.UnitID, "healingCostC1")
	if !known || unitCost < 0 || math.IsNaN(unitCost) || math.IsInf(unitCost, 0) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("official unit %d healingCostC1 is missing or malformed", request.UnitID), Localization.New("server.app.official_unit_p_healingcostc.b572772a", "official unit {p0} healingCostC1 is missing or malformed", Localization.Params{"p0": fmt.Sprintf("%d", request.UnitID)}))
	}
	amount, err := checkedCeilProduct(unitCost, request.Amount)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	return resolvedCoinCost{amount: amount, source: "official base healing cost", upperBound: true}, nil
}

func supportCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		SourceCastleID State.CastleID `json:"SID"`
		TargetX        int            `json:"TX"`
		TargetY        int            `json:"TY"`
		HorseID        int64          `json:"HBW"`
		Army           [][2]int64     `json:"A"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.SourceCastleID <= 0 || len(request.Army) == 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved support command is malformed"), Localization.New("server.app.resolved_support_command_is.aea1c2ff", "resolved support command is malformed", nil))
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("support source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.support_source_castle_p.f83898c1", "support source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	items, err := pairAmountSum(request.Army)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	distance := math.Hypot(float64(request.TargetX-source.X), float64(request.TargetY-source.Y))
	base, err := travelBaseCost(distance, items)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	horse, err := horseCoinCost(input.GameData, request.HorseID, distance, items)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	if horse < 0 || base > math.MaxInt64-horse {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("support travel cost overflowed"), Localization.New("server.app.support_travel_cost_overflowed.3b5fc7da", "support travel cost overflowed", nil))
	}
	return resolvedCoinCost{amount: base + horse, source: "official support travel formula", upperBound: true}, nil
}

func attackCoinCost(input Intent.PlanningContext, payload json.RawMessage) (resolvedCoinCost, error) {
	var request struct {
		attackBody
		AttackCount int `json:"AAC"`
		// Commander zero is valid; a pointer distinguishes it from missing or null LID.
		Leader *State.CommanderID `json:"LID"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.Leader == nil || *request.Leader < 0 || len(request.Waves) == 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("resolved attack command is malformed"), Localization.New("server.app.resolved_attack_command_is.6f3a11d8", "resolved attack command is malformed", nil))
	}
	body := request.attackBody
	items := int64(0)
	for _, wave := range body.Waves {
		for _, flank := range []attackFlank{wave.Left, wave.Middle, wave.Right} {
			for _, pairs := range [][]attackPair{flank.Units, flank.Tools} {
				for _, pair := range pairs {
					if pair[1] < 0 {
						return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack formation contains a negative amount"), Localization.New("server.app.attack_formation_contains_a.80c483c7", "attack formation contains a negative amount", nil))
					}
					if items > math.MaxInt64-pair[1] {
						return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack item count overflowed"), Localization.New("server.app.attack_item_count_overflowed.f9be1e26", "attack item count overflowed", nil))
					}
					items += pair[1]
				}
			}
		}
	}
	for _, pair := range body.SupportTroops {
		if pair[1] < 0 {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack support contains a negative amount"), Localization.New("server.app.attack_support_contains_a.7a32c8f3", "attack support contains a negative amount", nil))
		}
		if items > math.MaxInt64-pair[1] {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack item count overflowed"), Localization.New("server.app.attack_item_count_overflowed.f9be1e26", "attack item count overflowed", nil))
		}
		items += pair[1]
	}
	for _, toolID := range body.AttackSupportTools {
		if toolID > 0 {
			items++
		}
	}
	supportCount := int64(max(body.AttackSupportCount, 0))
	if items > math.MaxInt64-supportCount {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack item count overflowed"), Localization.New("server.app.attack_item_count_overflowed.f9be1e26", "attack item count overflowed", nil))
	}
	items += supportCount
	if items <= 0 {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack formation contains no countable items"), Localization.New("server.app.attack_formation_contains_no.9dae800c", "attack formation contains no countable items", nil))
	}
	distance := math.Hypot(float64(body.TargetX-body.SourceX), float64(body.TargetY-body.SourceY))
	base, err := travelBaseCost(distance, items)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	daily := input.State.DailyAttacks
	if daily.ObservedAt.IsZero() || daily.ConnectionGeneration != input.State.Session.ConnectionGeneration ||
		daily.Count < 0 || daily.ServerThreshold < 0 || daily.GrowthRate < 0 || math.IsNaN(daily.GrowthRate) || math.IsInf(daily.GrowthRate, 0) {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("current daily attack surcharge state is missing or malformed"), Localization.New("server.app.current_daily_attack_surcharge.7e3bfcfc", "current daily attack surcharge state is missing or malformed", nil))
	}
	total := float64(base)
	if daily.Count > daily.ServerThreshold {
		factor := math.Exp(daily.GrowthRate*float64(daily.Count-daily.ServerThreshold)) - 1
		if math.IsInf(factor, 0) || math.IsNaN(factor) {
			return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("daily attack surcharge overflowed"), Localization.New("server.app.daily_attack_surcharge_overflowed.3289b6a1", "daily attack surcharge overflowed", nil))
		}
		total = math.Min(factor*float64(base)+float64(base), float64(math.MaxInt32))
	}
	horsePerAttack, err := horseCoinCost(input.GameData, int64(body.Booster), distance, items)
	if err != nil {
		return resolvedCoinCost{}, err
	}
	horse := horsePerAttack
	if total >= math.Exp2(63) || float64(horse) >= math.Exp2(63)-total {
		return resolvedCoinCost{}, Localization.WithError(fmt.Errorf("attack travel cost overflowed"), Localization.New("server.app.attack_travel_cost_overflowed.99abe0fa", "attack travel cost overflowed", nil))
	}
	return resolvedCoinCost{amount: int64(math.Ceil(total)) + horse, source: "official attack travel and daily surcharge formula", upperBound: true}, nil
}

func travelBaseCost(distance float64, items int64) (int64, error) {
	if distance < 0 || math.IsNaN(distance) || math.IsInf(distance, 0) || items <= 0 {
		return 0, Localization.WithError(fmt.Errorf("travel distance or item count is malformed"), Localization.New("server.app.travel_distance_or_item.59dc9a21", "travel distance or item count is malformed", nil))
	}
	value := .6 * float64(items) * math.Log(distance+1) / math.Log(2.3)
	if math.IsNaN(value) || math.IsInf(value, 0) || value >= math.Exp2(63) {
		return 0, Localization.WithError(fmt.Errorf("travel cost overflowed"), Localization.New("server.app.travel_cost_overflowed.05d7a0ab", "travel cost overflowed", nil))
	}
	return max(int64(0), int64(math.Ceil(value))), nil
}

func horseCoinCost(store *GameData.Store, horseID int64, distance float64, items int64) (int64, error) {
	if horseID <= 0 {
		return 0, nil
	}
	if store == nil {
		return 0, Localization.WithError(fmt.Errorf("official game data is unavailable for horse %d", horseID), Localization.New("server.app.official_game_data_is.6a718478", "official game data is unavailable for horse {p0}", Localization.Params{"p0": fmt.Sprintf("%d", horseID)}))
	}
	catalog, err := store.Catalog("horses")
	if err != nil {
		return 0, err
	}
	raw, found := catalog.Find(strconv.FormatInt(horseID, 10))
	if !found {
		return 0, Localization.WithError(fmt.Errorf("official horse %d is unavailable", horseID), Localization.New("server.app.official_horse_p_is.74cbc49f", "official horse {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", horseID)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, err
	}
	factor, known := record.Float64("costFactorC1")
	if !known || factor < 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return 0, Localization.WithError(fmt.Errorf("official horse %d costFactorC1 is missing or malformed", horseID), Localization.New("server.app.official_horse_p_costfactorc.9baaa39b", "official horse {p0} costFactorC1 is missing or malformed", Localization.Params{"p0": fmt.Sprintf("%d", horseID)}))
	}
	base := math.Ceil(float64(items) * math.Log(distance+1) / math.Log(2.3) * .2)
	value := factor * base
	if value >= math.Exp2(63) {
		return 0, Localization.WithError(fmt.Errorf("horse travel cost overflowed"), Localization.New("server.app.horse_travel_cost_overflowed.0dc61fe5", "horse travel cost overflowed", nil))
	}
	return int64(value), nil
}

func pairAmountSum(pairs [][2]int64) (int64, error) {
	total := int64(0)
	for _, pair := range pairs {
		if pair[0] <= 0 || pair[1] <= 0 || total > math.MaxInt64-pair[1] {
			return 0, Localization.WithError(fmt.Errorf("travel manifest is malformed"), Localization.New("server.app.travel_manifest_is_malformed.6b3d8628", "travel manifest is malformed", nil))
		}
		total += pair[1]
	}
	return total, nil
}

func checkedCeilProduct(unitCost float64, amount int64) (int64, error) {
	value := unitCost * float64(amount)
	if amount <= 0 || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value >= math.Exp2(63) {
		return 0, Localization.WithError(fmt.Errorf("coin cost overflowed"), Localization.New("server.app.coin_cost_overflowed.76559520", "coin cost overflowed", nil))
	}
	return int64(math.Ceil(value)), nil
}
