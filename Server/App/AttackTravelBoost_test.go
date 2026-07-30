package App

import "testing"

func TestHorseTravelBoostFields(t *testing.T) {
	for _, test := range []struct {
		input       int
		wantBooster int
		wantTravel  int
	}{
		{input: -1, wantBooster: -1, wantTravel: 1},
		{input: 0, wantBooster: -1, wantTravel: 1},
		{input: 1007, wantBooster: 1007, wantTravel: 0},
		{input: 1008, wantBooster: 1008, wantTravel: 0},
		{input: 1009, wantBooster: 1009, wantTravel: 0},
	} {
		booster, travel := horseTravelBoostFields(test.input)
		if booster != test.wantBooster || travel != test.wantTravel {
			t.Fatalf("horseTravelBoostFields(%d) = (%d, %d), want (%d, %d)", test.input, booster, travel, test.wantBooster, test.wantTravel)
		}
	}
}
