package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models"
)

func TestApplyCastleResourceAmountsFromPayload(t *testing.T) {
	gs := &Models.GameState{}
	gs.Castle.MainCastle.Aid = 1234

	payload := `{"grc":{"AID":1234,"W":100,"S":200,"F":300,"C":400,"O":500,"G":600,"I":700,"HONEY":800,"MEAD":900,"BEEF":1000}}`
	if !ApplyCastleResourceAmountsFromPayload(gs, payload) {
		t.Fatalf("ApplyCastleResourceAmountsFromPayload changed = false, want true")
	}
	c := gs.GetCastleByID(1234)
	if c == nil {
		t.Fatalf("castle missing after setup")
	}
	if c.Amount.WoodAmount != 100 || c.Amount.StoneAmount != 200 || c.Amount.MeadAmount != 900 {
		t.Fatalf("amounts = %+v, want parsed grc values", c.Amount)
	}
	if ApplyCastleResourceAmountsFromPayload(gs, payload) {
		t.Fatalf("second ApplyCastleResourceAmountsFromPayload changed = true, want false")
	}
}
