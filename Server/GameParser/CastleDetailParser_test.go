package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models"
)

func testGCLCastleRow(cType, x, y, aid int, name string) map[string]interface{} {
	return map[string]interface{}{
		keyCastleInfoArray: []interface{}{
			float64(cType),
			float64(x),
			float64(y),
			float64(aid),
			float64(1),
			float64(1),
			float64(1),
			float64(1),
			float64(1),
			float64(1),
			name,
		},
	}
}

func TestParseGCLSpecialMetropolisAndCapitalRows(t *testing.T) {
	gs := Models.GetGameState()
	gs.Reset()

	gcl := map[string]interface{}{
		keyKingdoms: []interface{}{
			map[string]interface{}{
				keyKingdomID: float64(0),
				keyCastleInfoArray: []interface{}{
					testGCLCastleRow(CastleSlotMetropolis, 643, 410, 4760423, "WHITE BOY"),
					testGCLCastleRow(CastleSlotMain, 645, 410, 7869482, "SUMMER"),
					testGCLCastleRow(CastleSlotOutpost, 73, 369, 13278886, "VILLAINOUS"),
				},
			},
			map[string]interface{}{
				keyKingdomID: float64(2),
				keyCastleInfoArray: []interface{}{
					testGCLCastleRow(CastleSlotCapital, 856, 856, 4760132, "NEUSCHWANSTEIN"),
					testGCLCastleRow(CastleSlotForeign, 696, 705, 8144355, "DEBTMAXXING"),
				},
			},
		},
	}

	if err := parseGCL(gcl); err != nil {
		t.Fatalf("parseGCL returned error: %v", err)
	}

	if got := int(gs.Castle.Metropolis.Aid); got != 4760423 {
		t.Fatalf("metropolis aid = %d, want 4760423", got)
	}
	if gs.Castle.Metropolis.Name != "WHITE BOY" || gs.Castle.Metropolis.MapKingdomID != 0 || gs.Castle.Metropolis.MapX != 643 || gs.Castle.Metropolis.MapY != 410 {
		t.Fatalf("metropolis slot = %+v", gs.Castle.Metropolis)
	}
	if got := int(gs.Castle.Capital.Aid); got != 4760132 {
		t.Fatalf("capital aid = %d, want 4760132", got)
	}
	if gs.Castle.Capital.Name != "NEUSCHWANSTEIN" || gs.Castle.Capital.MapKingdomID != 2 || gs.Castle.Capital.MapX != 856 || gs.Castle.Capital.MapY != 856 {
		t.Fatalf("capital slot = %+v", gs.Castle.Capital)
	}
	if got := int(gs.Castle.IceCastle.Aid); got != 8144355 {
		t.Fatalf("ice aid = %d, want 8144355", got)
	}
}

func TestParseDCLAppliesSpecialCastleResourcesByAID(t *testing.T) {
	gs := Models.GetGameState()
	gs.Reset()
	gs.Castle.Capital.Aid = 4760132

	dcl := map[string]interface{}{
		keyKingdoms: []interface{}{
			map[string]interface{}{
				keyKingdomID: float64(2),
				keyCastleInfoArray: []interface{}{
					map[string]interface{}{
						keyCastleID: float64(4760132),
						keyW:        float64(100),
						keyS:        float64(200),
						keyF:        float64(300),
					},
				},
			},
		},
	}

	if err := parseDCL(dcl); err != nil {
		t.Fatalf("parseDCL returned error: %v", err)
	}
	if gs.Castle.Capital.Amount.WoodAmount != 100 || gs.Castle.Capital.Amount.StoneAmount != 200 || gs.Castle.Capital.Amount.FoodAmount != 300 {
		t.Fatalf("capital resources = %+v, want parsed DCL values", gs.Castle.Capital.Amount)
	}
}
