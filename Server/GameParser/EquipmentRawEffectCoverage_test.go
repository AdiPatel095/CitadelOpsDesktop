package GameParser

import (
	"testing"

	equip "CitadelDesktop/Server/Models/Equipment"
)

func statRow(id float64) []interface{} {
	return []interface{}{id, []interface{}{1.0}}
}

func TestRawEquipmentEffectCoverageFromGLI_commanderMissing(t *testing.T) {
	gli := map[string]interface{}{
		"C": []interface{}{
			map[string]interface{}{
				"EQ": []interface{}{
					[]interface{}{
						float64(1), float64(1), float64(2), float64(1), nil,
						[]interface{}{statRow(1), statRow(999001)},
						nil, nil, float64(1),
					},
				},
			},
		},
	}
	r := RawEquipmentEffectCoverageFromGLI(gli)
	if len(r.CastMissing) != 0 || len(r.GemStashMissing) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
	if len(r.CommMissing) != 1 || r.CommMissing[0] != 999001 {
		t.Fatalf("comm missing: %+v", r.CommMissing)
	}
}

func TestRawEquipmentEffectCoverageFromGLI_castMissing(t *testing.T) {
	gli := map[string]interface{}{
		"B": []interface{}{
			map[string]interface{}{
				"EQ": []interface{}{
					[]interface{}{
						float64(1), float64(1), float64(1), float64(1), nil,
						[]interface{}{statRow(10005), statRow(999002)},
						nil, nil, float64(1),
					},
				},
			},
		},
	}
	r := RawEquipmentEffectCoverageFromGLI(gli)
	if len(r.CommMissing) != 0 || len(r.GemStashMissing) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
	if len(r.CastMissing) != 1 || r.CastMissing[0] != 999002 {
		t.Fatalf("cast missing: %+v", r.CastMissing)
	}
}

func TestRawEquipmentEffectCoverageFromGemStorageMap_eitherMap(t *testing.T) {
	m := map[string]interface{}{
		"RGEM": []interface{}{
			[]interface{}{
				float64(1), float64(5), float64(0), float64(0),
				[]interface{}{statRow(1), statRow(10005), statRow(999003)},
				float64(1),
			},
		},
	}
	r := RawEquipmentEffectCoverageFromGemStorageMap(m)
	if len(r.CommMissing) != 0 || len(r.CastMissing) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
	if len(r.GemStashMissing) != 1 || r.GemStashMissing[0] != 999003 {
		t.Fatalf("stash: %+v", r.GemStashMissing)
	}
}

func TestRawEquipmentEffectCoverageFromGLI_slottedGemUsesCommContext(t *testing.T) {
	// Rarity 5, expanded gem at index 12 (same layout as ProcessGemSlot / ProcessGem).
	eqRow := []interface{}{
		float64(1), float64(1), float64(2), float64(5), nil,
		[]interface{}{statRow(1)},
		nil, nil, float64(1), nil, nil, nil,
		[]interface{}{
			float64(1), nil, nil,
			[]interface{}{
				float64(1), float64(1), float64(0), float64(0),
				[]interface{}{statRow(999004)},
				float64(1),
			},
		},
	}
	gli := map[string]interface{}{
		"C": []interface{}{
			map[string]interface{}{"EQ": []interface{}{eqRow}},
		},
	}
	r := RawEquipmentEffectCoverageFromGLI(gli)
	if len(r.CastMissing) != 0 || len(r.GemStashMissing) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
	if len(r.CommMissing) != 1 || r.CommMissing[0] != 999004 {
		t.Fatalf("gem should use comm context: %+v", r.CommMissing)
	}
}

func TestRawEquipmentEffectCoverageFromStorageMap_byEquipType(t *testing.T) {
	m := map[string]interface{}{
		"I": []interface{}{
			[]interface{}{
				float64(1), float64(1), float64(2), float64(1), nil,
				[]interface{}{statRow(999005)},
				nil, nil, float64(1),
			},
			[]interface{}{
				float64(2), float64(1), float64(1), float64(1), nil,
				[]interface{}{statRow(999006)},
				nil, nil, float64(1),
			},
		},
	}
	r := RawEquipmentEffectCoverageFromStorageMap(m)
	if len(r.GemStashMissing) != 0 {
		t.Fatalf("unexpected stash: %+v", r)
	}
	if len(r.CommMissing) != 1 || r.CommMissing[0] != 999005 {
		t.Fatalf("comm: %+v", r.CommMissing)
	}
	if len(r.CastMissing) != 1 || r.CastMissing[0] != 999006 {
		t.Fatalf("cast: %+v", r.CastMissing)
	}
}

func TestRawEquipmentEffectCoverage_empty(t *testing.T) {
	if !RawEquipmentEffectCoverageFromGLI(nil).EffectCoverageOK() {
		t.Fatal()
	}
	if !RawEquipmentEffectCoverageFromGLI(map[string]interface{}{}).EffectCoverageOK() {
		t.Fatal()
	}
}

func TestEffectCoverageReportMerge(t *testing.T) {
	a := equip.EffectCoverageReport{CommMissing: []float64{1}}
	b := equip.EffectCoverageReport{CommMissing: []float64{2}}
	m := a.Merge(b)
	if len(m.CommMissing) != 2 || m.CommMissing[0] != 1 || m.CommMissing[1] != 2 {
		t.Fatalf("%+v", m)
	}
}
