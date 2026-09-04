package AttackPresets

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
)

func TestValidateToolCompatibilityUsesOfficialTargetAndEventRestrictions(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":25,"name":"SamuraiRam","slotTypes":"1","allowedToAttack":"0+29","usageEventID":"80"},
			{"wodID":244,"name":"Eventtool","type":"EmperorKhanChest","comment2":"EmperorKhanChest","slotTypes":"1","allowedToAttack":"0+27#0+35","usageEventID":"5,72"},
			{"wodID":651,"name":"SceatSupport","slotTypes":"9"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	samuraiRam, khanChest, unrestricted := int64(25), int64(244), int64(651)
	target := ToolTarget{KingdomID: 0, TypeID: 29, EventID: 80, Label: "Samurai camps"}

	compatible := Preset{ID: "samurai", Name: "Samurai", Waves: []Wave{{
		Middle: Lane{Tools: []Slot{{ItemID: &samuraiRam, Quantity: 10}}},
	}}, CourtyardSupport: CourtyardSupport{Tools: []Slot{{ItemID: &unrestricted, Quantity: 1}}}}
	if err := ValidateToolCompatibility(compatible, gameData, target); err != nil {
		t.Fatalf("compatible Samurai preset rejected: %v", err)
	}

	incompatible := compatible
	incompatible.Name = "Sami's"
	incompatible.Waves = []Wave{{Middle: Lane{Tools: []Slot{{ItemID: &khanChest, Quantity: 10}}}}}
	err = ValidateToolCompatibility(incompatible, gameData, target)
	if err == nil {
		t.Fatal("Khan-only tool was accepted for a Samurai camp")
	}
	for _, expected := range []string{"Sami's", "Emperor Khan Chest", "Samurai camps", "remove or replace"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("compatibility error %q does not contain %q", err, expected)
		}
	}
}

func TestValidateToolCompatibilityFailsClosedOnMalformedOfficialRestriction(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":244,"name":"RestrictedTool","slotTypes":"1","allowedToAttack":"Samurai"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	toolID := int64(244)
	err = ValidateToolCompatibility(Preset{
		ID: "invalid", Name: "Invalid", Waves: []Wave{{Middle: Lane{Tools: []Slot{{ItemID: &toolID, Quantity: 1}}}}},
	}, gameData, ToolTarget{KingdomID: 0, TypeID: 29, EventID: 80, Label: "Samurai camps"})
	if err == nil || !strings.Contains(err.Error(), "unsupported allowedToAttack") {
		t.Fatalf("malformed official restriction error = %v", err)
	}
}
