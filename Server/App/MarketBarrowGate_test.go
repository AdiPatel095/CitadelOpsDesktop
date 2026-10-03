package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"
)

func marketBarrowTestInput(t *testing.T) Intent.PlanningContext {
	t.Helper()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"constructionItems":[],"effects":[],"levelBoosters":[],"horses":[],"buildings":[{"wodID":137,"name":"Market","marketCarriages":5}],"resources":[{"resourceID":12,"JSONKey":"F"},{"resourceID":18,"JSONKey":"MEAD"}]}`), GameData.SourceMetadata{ItemVersion: "synthetic-cit127"})
	if err != nil {
		t.Fatal(err)
	}
	gs := State.NewGameState()
	gs.Session.ChangedAt = time.Now().UTC().Add(-time.Hour)
	gs.Player.ID = 1
	gs.Market.CaravanLevelLoaded = true
	for _, id := range []State.CastleID{10, 20, 30} {
		castle := resourceIntentCastle(id, 0, int(id), int(id))
		castle.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
		castle.Resources[12] = State.ResourceBalance{Amount: 50000}
		castle.Resources[18] = State.ResourceBalance{Amount: 50000}
		gs.Castles[id] = castle
	}
	return Intent.PlanningContext{State: gs, GameData: data}
}

func TestMarketBarrowStaleSourceReplay(t *testing.T) {
	// R6 shape: two independent CRM goods shipments, 3905 food and 156 mead.
	for _, good := range []struct {
		resource int
		amount   int64
	}{{12, 3905}, {18, 156}} {
		t.Run(fmt.Sprint(good.resource), func(t *testing.T) {
			input := marketBarrowTestInput(t)
			now := time.Now().UTC()
			r := now.Add(time.Minute)
			input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, TotalBarrows: 125, AvailableBarrows: 125, ObservedAt: now.Add(-3 * time.Minute)}
			input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 30, Barrows: 125, ReleasesAt: r}}
			args := json.RawMessage(fmt.Sprintf(`{"sourceCastleId":10,"targetCastleId":20,"resourceId":%d,"amount":%d}`, good.resource, good.amount))
			plan, err := planMarketResourceShipment(t.Context(), input, args)
			if err != nil {
				t.Fatal(err)
			}
			cmi, crm := -1, -1
			for i, step := range plan.Steps {
				if step.Opcode == "cmi" {
					cmi = i
				}
				if step.Opcode == "crm" {
					crm = i
				}
			}
			if cmi < 0 || crm <= cmi || !slices.Contains(plan.Claims, "castle-focus") {
				t.Fatalf("refresh not before shipment: %+v", plan)
			}
			payload := string(plan.Steps[crm].Payload)
			row := input.State.Market.Castles[10]
			row.ObservedAt = time.Now().UTC()
			row.AvailableBarrows = 0
			input.State.Market.Castles[10] = row
			if _, err := planMarketResourceShipment(t.Context(), input, args); err == nil {
				t.Fatal("fresh shortage accepted")
			}
			required, err := marketBarrowsRequired(input, 10, good.amount)
			if err != nil || required <= 0 {
				t.Fatalf("required=%d err=%v", required, err)
			}
			row.AvailableBarrows = required
			input.State.Market.Castles[10] = row
			fresh, err := planMarketResourceShipment(t.Context(), input, args)
			if err != nil {
				t.Fatal(err)
			}
			if len(fresh.Steps) != 1 || fresh.Steps[0].Opcode != "crm" || string(fresh.Steps[0].Payload) != payload {
				t.Fatalf("fresh shipment changed: %+v", fresh)
			}
		})
	}
}

func TestMarketBarrowRequiredRoundsUpAndFailsClosed(t *testing.T) {
	input := marketBarrowTestInput(t)
	input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, ObservedAt: time.Now().UTC()}
	capacity, err := input.GameData.MarketCapacity(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		amount int64
		want   int
	}{{1, 1}, {int64(capacity.CapacityPerBarrow), 1}, {int64(capacity.CapacityPerBarrow) + 1, 2}} {
		got, err := marketBarrowsRequired(input, 10, tc.amount)
		if err != nil || got != tc.want {
			t.Fatalf("amount=%d got=%d err=%v", tc.amount, got, err)
		}
	}
	input.State.Market.CaravanLevelLoaded = false
	if _, err := marketBarrowsRequired(input, 10, 1); err == nil {
		t.Fatal("unobserved capacity accepted")
	}
}

func TestMarketBarrowRecordsDoNotScheduleMovementClock(t *testing.T) {
	input := marketBarrowTestInput(t)
	input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 125, ReleasesAt: time.Now().Add(-time.Minute)}}
	if got := nextMovementCompletion(input.State); !got.IsZero() {
		t.Fatalf("record scheduled past completion: %s", got)
	}
}

func TestMarketBarrowDispatchEvidenceCountsAndPrivacy(t *testing.T) {
	input := marketBarrowTestInput(t)
	now := time.Now().UTC()
	r := now.Add(-time.Second)
	input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, ObservedAt: now.Add(-time.Minute), TotalBarrows: 125, AvailableBarrows: 125}
	input.State.Market.ObservedAt = now.Add(-time.Minute)
	input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 125, ReleasesAt: r}}
	step := commandStep("Synthetic market shipment", "crm", json.RawMessage(`{"SID":10,"TX":87654323,"TY":87654324,"G":[["F",3905]],"PTT":0}`), "crm")
	raw, err := json.Marshal(captureDispatchBoundaryEvidence(t.Context(), input, step, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Market struct {
			SourceOrdinal, RequiredBarrows, ComputedAvailable int
			Lease                                             State.MarketBarrowLease
			Ready                                             bool
		}
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	required, _ := marketBarrowsRequired(input, 10, 3905)
	if value.Market.SourceOrdinal != 1 || value.Market.RequiredBarrows != required || value.Market.ComputedAvailable != 0 || value.Market.Lease.AwaitingConfirmation != 125 || !value.Market.Ready {
		t.Fatalf("evidence=%s", raw)
	}
}

func TestMarketBarrowCRM109StillLocksLane(t *testing.T) {
	step := boundaryTestStep("crm")
	engine, _, _ := boundaryTestEngine(t, boundaryTestState(), step, "", 109)
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "automation:autoFoodBalance", AutomationLane: "economy"})
	if len(receipt.Evidence) != 1 || !engine.AutomationLaneLock("economy").Active(time.Now()) {
		t.Fatalf("receipt=%+v", receipt)
	}
}
