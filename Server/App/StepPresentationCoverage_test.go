package App

import (
	"CitadelDesktop/Server/GameData"
	"strings"
	"testing"
)

func TestSpecialistRenewalStepNamesUseWholeRoleTemplates(t *testing.T) {
	for _, id := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10} {
		descriptor := specialistRenewalDescriptor(GameData.AutoBuyerSpecialist{ID: id, Name: "legacy role {raw}"})
		if descriptor == nil || !strings.HasPrefix(descriptor.Fallback, "Renew ") || strings.Contains(descriptor.Fallback, "legacy") || len(descriptor.Params) != 0 {
			t.Fatalf("specialist %d missing finite step template: %#v", id, descriptor)
		}
	}
	if specialistRenewalDescriptor(GameData.AutoBuyerSpecialist{ID: 999}) != nil {
		t.Fatal("unknown role inferred")
	}
}
