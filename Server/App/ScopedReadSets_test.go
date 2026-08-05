package App

import (
	"testing"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestRiftReadSetsCoverCurrentEngineState(t *testing.T) {
	input := Intent.PlanningContext{State: State.NewGameState()}
	tests := []struct {
		name         string
		resolve      Intent.ReadSetResolver
		capabilities []string
	}{
		{
			name: "maiden", resolve: riftMaidenReadSet,
			capabilities: []string{
				State.CapabilitySessionContext, State.CapabilityCastleDirectory,
				State.CapabilityBuildings, State.CapabilityGarrison, State.CapabilityLeaders,
				State.CapabilityEquipment, State.CapabilityWorldMap, State.CapabilityEvents,
			},
		},
		{
			name: "replay", resolve: riftReplayReadSet,
			capabilities: []string{
				State.CapabilitySessionContext, State.CapabilityCastleDirectory,
				State.CapabilityBuildings, State.CapabilityGarrison, State.CapabilityLeaders,
				State.CapabilityEvents, State.CapabilityAutomation,
			},
		},
		{name: "rename", resolve: riftTemplateReadSet, capabilities: []string{State.CapabilityEvents}},
		{
			name: "delete", resolve: riftTemplateDeleteReadSet,
			capabilities: []string{State.CapabilityEvents, State.CapabilityAutomation},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keys, err := test.resolve(input, nil, Intent.Plan{})
			if err != nil {
				t.Fatal(err)
			}
			if len(keys) != len(test.capabilities) {
				t.Fatalf("read set has %d keys, want %d: %#v", len(keys), len(test.capabilities), keys)
			}
			found := make(map[string]bool, len(keys))
			for _, key := range keys {
				found[key.Capability] = true
			}
			for _, capability := range test.capabilities {
				if !found[capability] {
					t.Errorf("read set is missing %q: %#v", capability, keys)
				}
			}
		})
	}
}
