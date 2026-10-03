package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/CommanderFeatures"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const premiumSupportCommander = -14

type supportCommanderReservation struct {
	operation, sendKey, feature string
	commander                   State.CommanderID
	generation, connection      uint64
	dispatchedAt                time.Time
}

// One registry per account runtime. Sent or uncertain premium movements remain
// debited until a newer authoritative VIP observation accounts for their use.
type premiumCommanderDispatchGate struct {
	mu          sync.Mutex
	next        uint64
	pending     map[string]supportCommanderReservation
	watermark   State.VIPState
	assignments func() (CommanderFeatures.Configuration, error)
	holds       Intent.CommanderHoldRegistry
}

func newPremiumCommanderDispatchGate() *premiumCommanderDispatchGate {
	return &premiumCommanderDispatchGate{pending: map[string]supportCommanderReservation{}}
}

func supportCommanderUnavailable(key string, params Localization.Params) error {
	return &Intent.SupportCommanderUnavailableError{Message: supportCommanderMessage(key, params)}
}

func currentSupportVIP(input Intent.PlanningContext) bool {
	vip, session := input.State.Player.VIP, input.State.Session
	now := time.Now().UTC()
	return session.ConnectionGeneration > 0 && vip.ConnectionGeneration == session.ConnectionGeneration &&
		vip.Generation == session.Generation && !vip.ObservedAt.IsZero() && !vip.ObservedAt.After(now) &&
		(vip.RemainingSec == 0 || vip.RemainingSec > 0 && now.Sub(vip.ObservedAt) < time.Duration(vip.RemainingSec)*time.Second) &&
		vip.Points >= 0 && vip.UsedPremiumCommanders >= 0
}

func supportPremiumAllowance(input Intent.PlanningContext) (int, bool) {
	if !currentSupportVIP(input) || input.GameData == nil {
		return 0, false
	}
	catalog, err := input.GameData.Catalog("viplevels")
	if err != nil {
		return 0, false
	}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		low, lowOK := record.Int64("thresholdMin")
		high, highOK := record.Int64("thresholdMax")
		free, freeOK := record.Int64("freePremiumGeneralsPerDay")
		if _, present := record["freePremiumGeneralsPerDay"]; !present {
			free, freeOK = 0, true
		}
		if lowOK && highOK && freeOK && free >= 0 && low <= input.State.Player.VIP.Points && input.State.Player.VIP.Points <= high {
			return int(free), true
		}
	}
	return 0, false
}

func (gate *premiumCommanderDispatchGate) reconcile(input Intent.PlanningContext) bool {
	// An authoritative movement refresh resolves owned-commander uncertainty.
	// Keep premium debits separate: GAM does not account for VIP daily usage.
	snapshot := input.State.MovementSnapshot
	for token, reservation := range gate.pending {
		if reservation.commander >= 0 && !reservation.dispatchedAt.IsZero() && snapshot.ConnectionGeneration == reservation.connection && snapshot.ObservedAt.After(reservation.dispatchedAt) {
			gate.release(token)
		}
	}
	vip := input.State.Player.VIP
	if !currentSupportVIP(input) {
		return false
	}
	previous := gate.watermark
	if !previous.ObservedAt.IsZero() {
		if vip.ConnectionGeneration < previous.ConnectionGeneration || vip.Generation < previous.Generation ||
			vip.ObservedAt.Before(previous.ObservedAt) ||
			vip.ObservedAt.Equal(previous.ObservedAt) && vip != previous {
			return false
		}
	}
	if vip.ConnectionGeneration != previous.ConnectionGeneration || vip.Generation != previous.Generation {
		// A new session snapshot is authoritative; old reservation tokens cannot send.
		for token, reservation := range gate.pending {
			if reservation.connection != vip.ConnectionGeneration || reservation.generation != vip.Generation {
				gate.release(token)
			}
		}
	} else if vip.ObservedAt.After(previous.ObservedAt) {
		increase := vip.UsedPremiumCommanders - previous.UsedPremiumCommanders
		var tokens []string
		for token, reservation := range gate.pending {
			if reservation.commander == premiumSupportCommander && !reservation.dispatchedAt.IsZero() && vip.ObservedAt.After(reservation.dispatchedAt) {
				tokens = append(tokens, token)
			}
		}
		sort.Slice(tokens, func(i, j int) bool {
			return gate.pending[tokens[i]].dispatchedAt.Before(gate.pending[tokens[j]].dispatchedAt)
		})
		for _, token := range tokens {
			if increase <= 0 {
				break
			}
			delete(gate.pending, token)
			increase--
		}
	}
	gate.watermark = vip
	return true
}

func (gate *premiumCommanderDispatchGate) reserve(input Intent.PlanningContext, commander State.CommanderID, feature string) string {
	if input.DryRun {
		return "dry-run"
	}
	gate.next++
	token := strconv.FormatUint(gate.next, 10)
	gate.pending[token] = supportCommanderReservation{operation: input.OperationID, sendKey: input.SupportSendKey, feature: feature, commander: commander, generation: input.State.Session.Generation, connection: input.State.Session.ConnectionGeneration}
	if commander >= 0 && input.CommanderHolds != nil {
		gate.holds = input.CommanderHolds
		input.CommanderHolds.HoldCommanders([]State.CommanderID{commander}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	return token
}

func (gate *premiumCommanderDispatchGate) existing(input Intent.PlanningContext) (State.CommanderID, string, bool) {
	if input.OperationID == "" || input.SupportSendKey == "" {
		return 0, "", false
	}
	for token, reservation := range gate.pending {
		if reservation.operation == input.OperationID && reservation.sendKey == input.SupportSendKey && reservation.dispatchedAt.IsZero() &&
			reservation.generation == input.State.Session.Generation && reservation.connection == input.State.Session.ConnectionGeneration {
			return reservation.commander, token, true
		}
	}
	return 0, "", false
}

func (gate *premiumCommanderDispatchGate) reservePremium(input Intent.PlanningContext) (string, bool) {
	allowance, known := supportPremiumAllowance(input)
	if !known || input.State.Player.VIP.RemainingSec <= 0 || !gate.reconcile(input) {
		return "", false
	}
	pending := 0
	for _, reservation := range gate.pending {
		if reservation.commander == premiumSupportCommander {
			pending++
		}
	}
	if allowance-input.State.Player.VIP.UsedPremiumCommanders-pending < 1 {
		return "", false
	}
	return gate.reserve(input, premiumSupportCommander, ""), true
}

func (gate *premiumCommanderDispatchGate) ReservePremiumCommander(input Intent.PlanningContext) (string, bool) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	allowance, known := supportPremiumAllowance(input)
	if !known || input.State.Player.VIP.RemainingSec <= 0 || !gate.reconcile(input) {
		return "", false
	}
	if commander, token, ok := gate.existing(input); ok && commander == premiumSupportCommander {
		pending := 0
		for _, r := range gate.pending {
			if r.commander == premiumSupportCommander {
				pending++
			}
		}
		if allowance-input.State.Player.VIP.UsedPremiumCommanders < pending {
			return "", false
		}
		return token, true
	}
	return gate.reservePremium(input)
}

func (gate *premiumCommanderDispatchGate) SelectSupportCommander(input Intent.PlanningContext, feature string, source State.CastleState) (State.CommanderID, string, error) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.reconcile(input)
	if commander, token, ok := gate.existing(input); ok {
		return commander, token, nil
	}
	if token, ok := gate.reservePremium(input); ok {
		return premiumSupportCommander, token, nil
	}
	settings := CommanderFeatures.Configuration{}
	if gate.assignments != nil {
		var err error
		settings, err = gate.assignments()
		if err != nil {
			return 0, "", supportCommanderUnavailable("server.support.commander_wait", nil)
		}
	}
	speedCap, costCap, ok := supportEffectCaps(input)
	if !ok {
		return 0, "", supportCommanderUnavailable("server.support.commander_wait", nil)
	}
	best := State.CommanderID(-1)
	bestSpeed, bestCost := -math.MaxFloat64, -math.MaxFloat64
	now := time.Now().UTC()
	for id, commander := range input.State.Commanders {
		if id < 0 || !commander.Available || State.CommanderHasActiveMovementAt(&input.State, id, now) || State.InvasionCommanderReserved(&input.State, id) ||
			!CommanderFeatures.AssignmentAllows(settings, feature, id) || !CommanderFeatures.MeetsFeatureRequirements(input.State, settings, feature, id) ||
			input.CommanderHolds != nil && input.CommanderHolds.CommanderHeldAt(id, now) {
			continue
		}
		held := false
		for _, reservation := range gate.pending {
			if reservation.commander == id {
				held = true
				break
			}
		}
		if held {
			continue
		}
		speed := min(supportEquipmentEffect(input.State, commander, 263), speedCap)
		cost := min(supportEquipmentEffect(input.State, commander, 265), costCap)
		if best < 0 || speed > bestSpeed || speed == bestSpeed && (cost > bestCost || cost == bestCost && id < best) {
			best, bestSpeed, bestCost = id, speed, cost
		}
	}
	if best < 0 {
		return 0, "", supportCommanderUnavailable("server.support.commander_wait", nil)
	}
	return best, gate.reserve(input, best, feature), nil
}

func supportEffectCaps(input Intent.PlanningContext) (float64, float64, bool) {
	if input.GameData == nil {
		return 0, 0, false
	}
	catalog, err := input.GameData.Catalog("effectCaps")
	if err != nil {
		return 0, 0, false
	}
	raw, ok := catalog.Find("99")
	if !ok {
		return 0, 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, 0, false
	}
	maximum, ok := record.Float64("maxTotalBonus")
	// Official cap 99 has no maxTotalBonus: applying that row leaves the total
	// uncapped. A numeric maximum, when present, is enforced on both effects.
	if _, present := record["maxTotalBonus"]; !present {
		return math.MaxFloat64, math.MaxFloat64, true
	}
	return maximum, maximum, ok && maximum >= 0 && !math.IsNaN(maximum) && !math.IsInf(maximum, 0)
}

func supportEquipmentEffect(state State.GameState, commander State.CommanderState, effectID int64) float64 {
	total := 0.0
	for _, id := range commander.Equipment {
		for _, effect := range state.Inventory.Equipment[id].Effects {
			if effect.DefinitionID == effectID && len(effect.Values) > 0 {
				value := effect.Values[len(effect.Values)-1]
				if !math.IsNaN(value) && !math.IsInf(value, 0) {
					total += value
				}
			}
		}
	}
	return total
}

func premiumCommanderPayload(payload json.RawMessage) bool {
	var fields struct {
		BPC int
		LID int
	}
	// Malformed premium fields cannot bypass the no-spend backstop.
	return json.Unmarshal(payload, &fields) != nil || fields.BPC == 1 || fields.LID == premiumSupportCommander
}

func (gate *premiumCommanderDispatchGate) Validate(ctx context.Context, input Intent.PlanningContext, step Intent.Step) error {
	opcode := strings.ToLower(step.Opcode)
	if opcode != "cds" && opcode != "cra" {
		return nil
	}
	premium := premiumCommanderPayload(step.Payload)
	if opcode == "cra" {
		if premium {
			return supportCommanderUnavailable("server.support.premium_blocked", nil)
		}
		return nil
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.reconcile(input)
	reservation, exists := gate.pending[step.SupportCommanderReservation]
	if !premium && !exists {
		return nil
	}
	metadata := Outbound.MetadataFromContext(ctx)
	if !exists || reservation.operation != metadata.OperationID || reservation.generation != input.State.Session.Generation || reservation.connection != input.State.Session.ConnectionGeneration || !reservation.dispatchedAt.IsZero() {
		return supportCommanderUnavailable("server.support.premium_blocked", nil)
	}
	var fields struct {
		LID State.CommanderID
		BPC int
	}
	if json.Unmarshal(step.Payload, &fields) != nil || fields.LID != reservation.commander || fields.BPC != supportPremiumFlag(premium) {
		return supportCommanderUnavailable("server.support.premium_blocked", nil)
	}
	if premium {
		allowance, known := supportPremiumAllowance(input)
		if !known || input.State.Player.VIP.RemainingSec <= 0 || !gate.reconcile(input) {
			return supportCommanderUnavailable("server.support.premium_blocked", nil)
		}
		pending := 0
		for _, r := range gate.pending {
			if r.commander == premiumSupportCommander {
				pending++
			}
		}
		if allowance-input.State.Player.VIP.UsedPremiumCommanders < pending {
			return supportCommanderUnavailable("server.support.premium_blocked", nil)
		}
	} else {
		commander, ok := input.State.Commanders[reservation.commander]
		settings := CommanderFeatures.Configuration{}
		var err error
		if gate.assignments != nil {
			settings, err = gate.assignments()
		}
		if err != nil || !ok || !commander.Available || State.CommanderHasActiveMovementAt(&input.State, reservation.commander, time.Now().UTC()) || State.InvasionCommanderReserved(&input.State, reservation.commander) ||
			!CommanderFeatures.AssignmentAllows(settings, reservation.feature, reservation.commander) || !CommanderFeatures.MeetsFeatureRequirements(input.State, settings, reservation.feature, reservation.commander) {
			return supportCommanderUnavailable("server.support.commander_wait", nil)
		}
	}
	reservation.dispatchedAt = time.Now().UTC()
	gate.pending[step.SupportCommanderReservation] = reservation
	return nil
}

func (gate *premiumCommanderDispatchGate) release(token string) {
	if reservation, ok := gate.pending[token]; ok && reservation.commander >= 0 {
		if holds, ok := gate.holds.(interface{ ReleaseCommanders([]State.CommanderID) }); ok {
			holds.ReleaseCommanders([]State.CommanderID{reservation.commander})
		}
	}
	delete(gate.pending, token)
}
func (gate *premiumCommanderDispatchGate) ReleaseSupportCommander(token string) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.release(token)
}
func (gate *premiumCommanderDispatchGate) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if reservation, ok := gate.pending[step.SupportCommanderReservation]; ok && reservation.operation == Outbound.MetadataFromContext(ctx).OperationID {
		gate.release(step.SupportCommanderReservation)
	}
}
func (*premiumCommanderDispatchGate) Indeterminate(context.Context, Intent.Step) {} // uncertain premium use must stay debited
func (gate *premiumCommanderDispatchGate) Completed(_ context.Context, input Intent.PlanningContext, step Intent.Step, _ Protocol.CommittedFrame) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if reservation, ok := gate.pending[step.SupportCommanderReservation]; ok && reservation.commander >= 0 {
		delete(gate.pending, step.SupportCommanderReservation)
		if holds, ok := gate.holds.(interface {
			CompleteSupportHold(State.CommanderID, time.Time)
		}); ok {
			holds.CompleteSupportHold(reservation.commander, time.Now().UTC().Add(craCommanderLaunchHold))
		}
	}
	gate.reconcile(input)
	return false
}
func (gate *premiumCommanderDispatchGate) OperationFinished(ctx context.Context) {
	metadata := Outbound.MetadataFromContext(ctx)
	gate.mu.Lock()
	defer gate.mu.Unlock()
	for token, reservation := range gate.pending {
		if reservation.operation == metadata.OperationID && reservation.dispatchedAt.IsZero() {
			gate.release(token)
		}
	}
}

func reservePremiumCommander(input Intent.PlanningContext, storm bool) (string, error) {
	if input.SupportCommanders != nil {
		if token, ok := input.SupportCommanders.ReservePremiumCommander(input); ok {
			return token, nil
		}
	}
	key := "server.support.manual_quota"
	if storm {
		key = "server.support.storm_quota"
	}
	if _, known := supportPremiumAllowance(input); !known {
		key = "server.support.manual_vip_unknown"
		if storm {
			key = "server.support.storm_vip_unknown"
		}
	}
	return "", supportCommanderUnavailable(key, nil)
}

func selectSupportCommander(input Intent.PlanningContext, feature string, source State.CastleState) (State.CommanderID, string, error) {
	if input.SupportCommanders == nil {
		return 0, "", supportCommanderUnavailable("server.support.commander_wait", nil)
	}
	return input.SupportCommanders.SelectSupportCommander(input, feature, source)
}

func supportCommanderFeature(input Intent.PlanningContext) string {
	if input.AutomationLane == "autoBird" || strings.HasPrefix(input.IntentName, "auto_bird.") {
		return "autoBird"
	}
	if input.AutomationLane == "autoStation" {
		return "autoStation"
	}
	return ""
}

// Source and destination distinguish batches when a resolver is retried.
func supportSendKey(source State.CastleID, target State.AllianceHolding, start int) string {
	return fmt.Sprintf("%d:%d:%d:%d", source, target.X, target.Y, start)
}

func supportPremiumFlag(premium bool) int {
	if premium {
		return 1
	}
	return 0
}
