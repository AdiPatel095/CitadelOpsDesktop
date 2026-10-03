package Automation

import (
	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func replayBeriKingdomData(t *testing.T, includeFactionStorage bool) *GameData.Store {
	t.Helper()
	raw, err := os.ReadFile("../Buildings/testdata/berimond_storage_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var records []json.RawMessage
	json.Unmarshal(fixture["buildings"], &records)
	raw, err = os.ReadFile("../GameData/testdata/default_building_kingdoms_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	var official struct {
		Buildings []json.RawMessage `json:"buildings"`
	}
	if err := json.Unmarshal(raw, &official); err != nil {
		t.Fatal(err)
	}
	// Use verbatim records for the two storage definitions, preserving the
	// existing bounded public event catalog for other camp buildings.
	kept := records[:0]
	for _, record := range records {
		var identity struct {
			ID int64 `json:"wodID"`
		}
		json.Unmarshal(record, &identity)
		if identity.ID != 132 && identity.ID != 246 {
			kept = append(kept, record)
		}
	}
	for _, record := range official.Buildings {
		var identity struct {
			ID int64 `json:"wodID"`
		}
		json.Unmarshal(record, &identity)
		if identity.ID == 132 || identity.ID == 246 && includeFactionStorage {
			kept = append(kept, record)
		}
	}
	kept = append(kept, json.RawMessage(`{"wodID":900001,"name":"ClassicOnlyDecoration","group":"Building","shopCategory":"DECO","width":2,"height":2,"costWood":1}`))
	fixture["buildings"], _ = json.Marshal(kept)
	raw, _ = json.Marshal(fixture)
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Sanitized replay of the September 23/24 EBU/177 sequence: kingdom 10,
// a storage shortfall, and ordinary Storehouse132 competing with FactionStorage.
// All state identities and geometry come from synthetic/public test helpers.
func TestAutoBeriStorageShortfallReplayUses246Never132(t *testing.T) {
	for _, date := range []string{"2026-09-23", "2026-09-24"} {
		t.Run(date, func(t *testing.T) {
			snapshot, settings, castle := beriOfficialBuildFixture(t, 17, 0)
			snapshot.Now, _ = time.Parse("2006-01-02", date)
			castle.Layout.ObservedAt, castle.BuildingQueue.ObservedAt = snapshot.Now, snapshot.Now
			snapshot.State.Castles[castle.ID] = castle
			snapshot.GameData = replayBeriKingdomData(t, true)
			dependency, err := Buildings.PreviewStorageDependency(snapshot.State, snapshot.GameData, Buildings.StorageDependencyRequest{
				CastleID: castle.ID, EventID: optionalAutoEventBuildID(GameData.BerimondEventID),
				Costs:                        []Buildings.CostStatus{{Scope: GameData.BuildingCostCastleResource, DefinitionID: 3, Key: "W", Required: 20000}},
				AllowedBuildingDefinitionIDs: []State.BuildingID{132, 246},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !dependency.Required || dependency.RecommendedAction == nil {
				t.Fatalf("storage replay blocked: %#v", dependency)
			}
			var args struct {
				DefinitionID State.BuildingID `json:"definitionId"`
			}
			encoded, _ := json.Marshal(dependency.RecommendedAction.Arguments)
			if err := json.Unmarshal(encoded, &args); err != nil {
				t.Fatal(err)
			}
			if args.DefinitionID != 246 {
				t.Fatalf("storage replay selected %d instead of246", args.DefinitionID)
			}
			target := []Buildings.TargetBuilding{{TargetID: "ordinary-store", DefinitionID: 132, Placement: &Buildings.TargetPlacement{GridX: 220, GridY: 180}}, {TargetID: "faction-store", DefinitionID: 246, Placement: &Buildings.TargetPlacement{GridX: 180, GridY: 180}}}
			diff, err := Buildings.CompileTargetDiff(snapshot.State, snapshot.GameData, Buildings.TargetDiffRequest{CastleID: castle.ID, EventID: optionalAutoEventBuildID(3), Buildings: target})
			if err != nil {
				t.Fatal(err)
			}
			decision, _, _, err := beriPhaseAction(snapshot, settings, castle, diff, map[string]float64{}, autoEventBuildProfile{KingdomID: 10, EventID: 3}, "")
			if err != nil || decision == nil || decision.Request == nil {
				t.Fatalf("phase decision=%#v err=%v", decision, err)
			}
			if err := json.Unmarshal(decision.Request.Arguments, &args); err != nil || args.DefinitionID != 246 {
				t.Fatalf("invalid Beri phase: %s %v", decision.Request.Arguments, err)
			}
		})
	}
}

func TestAutoBeriIneligibleStorageStatusContinuesOtherConstruction(t *testing.T) {
	snapshot, settings, castle := beriOfficialBuildFixture(t, 17, 0)
	snapshot.GameData = replayBeriKingdomData(t, true)
	target := Buildings.TargetCaptureResult{Version: 1, KingdomID: 10, Buildings: []Buildings.TargetBuilding{{TargetID: "ordinary", DefinitionID: 132}}}
	decision := Decision{Status: "waiting"}
	attachBuildingKingdomNotices(&decision, snapshot, castle, &target)
	if decision.Status != "blocked" || !strings.Contains(decision.Detail, "Storehouse") || !strings.Contains(decision.Detail, "kingdom 10") || decision.DetailDescriptor == nil || decision.Request != nil {
		t.Fatalf("missing local status: %#v", decision)
	}
	fingerprint, _ := passiveDecisionFingerprint(decision)
	attachBuildingKingdomNotices(&decision, snapshot, castle, &target)
	if next, _ := passiveDecisionFingerprint(decision); next != fingerprint {
		t.Fatal("unchanged notice was not deduplicatable")
	}
	castle.KingdomID = 11
	changed := Decision{Status: "waiting"}
	attachBuildingKingdomNotices(&changed, snapshot, castle, &target)
	if next, _ := passiveDecisionFingerprint(changed); next == fingerprint {
		t.Fatal("changed kingdom was hidden by deduplication")
	}
	castle.KingdomID = 10
	// An invalid storage target must not suppress valid final storage.
	settings.Target.Fixed = nil
	settings.Target.Buildings = []Buildings.TargetBuilding{{TargetID: "invalid-decoration", DefinitionID: 900001, Placement: &Buildings.TargetPlacement{GridX: 230, GridY: 180}}, {TargetID: "invalid-store", DefinitionID: 132, Placement: &Buildings.TargetPlacement{GridX: 220, GridY: 180}}, {TargetID: "storage", DefinitionID: 246, Placement: &Buildings.TargetPlacement{GridX: 180, GridY: 180}}}
	decisionPtr, _, detail, err := evaluateBeriEventBuild(snapshot, settings, castle, map[string]float64{}, autoEventBuildProfile{KingdomID: 10, EventID: 3})
	if err != nil || decisionPtr == nil || decisionPtr.Request == nil {
		t.Fatalf("eligible work stopped: %#v %v detail=%s", decisionPtr, err, detail)
	}
	var args struct {
		DefinitionID State.BuildingID `json:"definitionId"`
	}
	if err := json.Unmarshal(decisionPtr.Request.Arguments, &args); err != nil || args.DefinitionID != 246 {
		t.Fatalf("eligible work: %s %v", decisionPtr.Request.Arguments, err)
	}
	attachBuildingKingdomNotices(decisionPtr, snapshot, castle, settings.Target)
	if decisionPtr.Request == nil || decisionPtr.Details["buildingKingdomNotice/invalid-store"] == "" {
		t.Fatal("continuing work omitted invalid-building notice")
	}
}

func TestAutoStormDefaultKingdomStorehouseStillBuilds(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	target := Buildings.TargetCaptureResult{Version: 1, KingdomID: autoStormKingdomID, Buildings: []Buildings.TargetBuilding{{TargetID: "store", DefinitionID: 132}}}
	settings := strictStormSettings(target)
	snapshot := Snapshot{State: state, GameData: strictStormGameData(t), Now: now}
	catalog, err := snapshot.GameData.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	storehouse, _ := catalog.Definition(132)
	if !storehouse.KingdomIDsDefaulted || Buildings.BuildingKingdomBlocker(storehouse, castle.KingdomID) != nil {
		t.Fatal("Storm default storehouse became ineligible")
	}
	decision, _, detail, err := evaluateStrictAutoStormBuild(snapshot, settings, castle, map[string]float64{})
	if err != nil || decision == nil || decision.Request == nil || decision.Request.Name != "building.construct" {
		t.Fatalf("Storm regression decision=%#v detail=%s err=%v", decision, detail, err)
	}
}

func TestAutoBeriNoEligibleStorageIsBlocked(t *testing.T) {
	snapshot, _, castle := beriOfficialBuildFixture(t, 17, 0)
	snapshot.GameData = replayBeriKingdomData(t, false)
	dependency, err := Buildings.PreviewStorageDependency(snapshot.State, snapshot.GameData, Buildings.StorageDependencyRequest{
		CastleID: castle.ID, EventID: optionalAutoEventBuildID(3),
		Costs:                        []Buildings.CostStatus{{Scope: GameData.BuildingCostCastleResource, DefinitionID: 3, Key: "W", Required: 20000}},
		AllowedBuildingDefinitionIDs: []State.BuildingID{132},
	})
	if err != nil || !dependency.Required || dependency.RecommendedAction != nil {
		t.Fatalf("ineligible storage selected: %#v %v", dependency, err)
	}
	target := Buildings.TargetCaptureResult{KingdomID: 10, Buildings: []Buildings.TargetBuilding{{TargetID: "ordinary-store", DefinitionID: 132}}}
	decision := Decision{Status: "waiting"}
	attachBuildingKingdomNotices(&decision, snapshot, castle, &target)
	if decision.Status != "blocked" || decision.Request != nil || decision.DetailDescriptor == nil || len(snapshot.State.Automations) != 0 {
		t.Fatalf("storage did not block locally: %#v", decision)
	}
}
