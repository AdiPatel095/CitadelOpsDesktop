package Buildings

import (
	"fmt"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestDefaultBerimondTargetUsesReferenceLayoutAndMaximumCampWoDs(t *testing.T) {
	target, err := DefaultBerimondTarget(901, 3, defaultBerimondTargetStore(t, 0))
	if err != nil {
		t.Fatal(err)
	}
	if target.CastleID != 901 || target.KingdomID != State.KingdomID(GameData.BerimondKingdomID) ||
		target.Mode != TargetCaptureModeExact || !target.Exact {
		t.Fatalf("unexpected target identity: %#v", target)
	}
	if len(target.Ground) != 17 || len(target.Fixed) != 22 || len(target.Buildings) != 156 ||
		target.Summary.GroundCount != 17 || target.Summary.FixedCount != 22 ||
		target.Summary.BuildingCount != 92 || target.Summary.DecorationCount != 64 {
		t.Fatalf("unexpected reference target summary: %#v", target.Summary)
	}
	counts := targetDefinitionCounts(target.Buildings)
	for definitionID, want := range map[State.BuildingID]int{
		12:  82, // Maximum Small tents WoD.
		14:  2,  // Maximum Large tents WoD.
		626: 1,  // Maximum Auxiliaries' headquarters WoD.
		293: 1,  // Configured Stable level 3 WoD.
	} {
		if counts[definitionID] != want {
			t.Fatalf("definition %d count = %d, want %d", definitionID, counts[definitionID], want)
		}
	}
	for _, intermediate := range []State.BuildingID{11, 243, 244, 13, 245, 623, 624, 625, 294} {
		if counts[intermediate] != 0 {
			t.Fatalf("intermediate or snapshot-only definition %d remains in target: %#v", intermediate, counts)
		}
	}
	for _, building := range target.Buildings {
		if building.Placement == nil {
			t.Fatalf("exact target %q lost its reference placement", building.TargetID)
		}
	}
}

func TestDefaultBerimondTargetResolvesStableLevelsFromOfficialWoDs(t *testing.T) {
	store := defaultBerimondTargetStore(t, 0)
	for level, definitionID := range map[int64]State.BuildingID{
		1: 247,
		2: 15,
		3: 293,
		4: 16,
		5: 294,
	} {
		target, err := DefaultBerimondTarget(901, level, store)
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		if got := targetDefinitionCounts(target.Buildings)[definitionID]; got != 1 {
			t.Fatalf("level %d Stable WoD %d count = %d, want 1", level, definitionID, got)
		}
	}
	for _, level := range []int64{0, 6} {
		if _, err := DefaultBerimondTarget(901, level, store); err == nil {
			t.Fatalf("Stable level %d was accepted", level)
		}
	}
}

func TestDefaultBerimondTargetFollowsNewOfficialCampMaximum(t *testing.T) {
	target, err := DefaultBerimondTarget(901, 5, defaultBerimondTargetStore(t, 999))
	if err != nil {
		t.Fatal(err)
	}
	counts := targetDefinitionCounts(target.Buildings)
	if counts[999] != 82 || counts[12] != 0 || counts[11] != 0 || counts[243] != 0 {
		t.Fatalf("Small tents did not follow the official terminal WoD: %#v", counts)
	}
}

func targetDefinitionCounts(targets []TargetBuilding) map[State.BuildingID]int {
	result := map[State.BuildingID]int{}
	for _, target := range targets {
		result[target.DefinitionID]++
	}
	return result
}

func defaultBerimondTargetStore(t *testing.T, smallTentUpgrade int64) *GameData.Store {
	t.Helper()
	upgrade := ""
	extra := ""
	if smallTentUpgrade > 0 {
		upgrade = fmt.Sprintf(`,"upgradeWodID":"%d"`, smallTentUpgrade)
		extra = fmt.Sprintf(
			`,{"wodID":%d,"name":"FactionUnittent","level":"5","kIDs":"10","buildingGroundType":"BUILDING"}`,
			smallTentUpgrade,
		)
	}
	raw := fmt.Sprintf(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":233,"name":"FactionMaintent","level":"1","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":11,"name":"FactionUnittent","level":"2","upgradeWodID":"243","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":243,"name":"FactionUnittent","level":"3","upgradeWodID":"12","downgradeWodID":"11","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":12,"name":"FactionUnittent","level":"4","downgradeWodID":"243","kIDs":"10","buildingGroundType":"BUILDING"%s}%s,
			{"wodID":244,"name":"FactionPUnittent","level":"1","upgradeWodID":"13","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":13,"name":"FactionPUnittent","level":"2","upgradeWodID":"245","downgradeWodID":"244","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":245,"name":"FactionPUnittent","level":"3","upgradeWodID":"14","downgradeWodID":"13","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":14,"name":"FactionPUnittent","level":"4","downgradeWodID":"245","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":246,"name":"FactionStorage","level":"1","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":247,"name":"FactionStable","level":"1","upgradeWodID":"15","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":15,"name":"FactionStable","level":"2","upgradeWodID":"293","downgradeWodID":"247","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":293,"name":"FactionStable","level":"3","upgradeWodID":"16","downgradeWodID":"15","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":16,"name":"FactionStable","level":"4","upgradeWodID":"294","downgradeWodID":"293","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":294,"name":"FactionStable","level":"5","downgradeWodID":"16","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":336,"name":"FactionDeco","type":"FactionArmory","level":"1","kIDs":"10","buildingGroundType":"DECO"},
			{"wodID":337,"name":"FactionDeco","type":"FactionFieldkitchen","level":"1","kIDs":"10","buildingGroundType":"DECO"},
			{"wodID":339,"name":"FactionDeco","type":"FactionTrainingground","level":"1","kIDs":"10","buildingGroundType":"DECO"},
			{"wodID":468,"name":"FactionHospital","level":"1","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":623,"name":"FactionUnitCamp","level":"2","upgradeWodID":"624","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":624,"name":"FactionUnitCamp","level":"3","upgradeWodID":"625","downgradeWodID":"623","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":625,"name":"FactionUnitCamp","level":"4","upgradeWodID":"626","downgradeWodID":"624","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":626,"name":"FactionUnitCamp","level":"5","downgradeWodID":"625","kIDs":"10","buildingGroundType":"BUILDING"},
			{"wodID":627,"name":"FactionBarracks","level":"1","kIDs":"10","buildingGroundType":"BUILDING"}
		],
		"units":[]
	}`, upgrade, extra)
	store, err := GameData.DecodeStore([]byte(raw), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
