package equipment

import "testing"

func TestCommander2026SalesSetBonusMapsReinforcement(t *testing.T) {
	var stat CommStatModel

	ApplyCommanderSetBonusStats(&stat, 1096, 9)

	if stat.AttackReinforcement != 1500 {
		t.Fatalf("AttackReinforcement = %v, want 1500", stat.AttackReinforcement)
	}
	if stat.Wave != 3 {
		t.Fatalf("Wave = %v, want 3", stat.Wave)
	}
	var foundReinforcement bool
	for _, effect := range stat.Effects {
		if effect.EffectID == 483 {
			foundReinforcement = true
			if effect.Source != CatalogEffectSourceSetBonus || effect.Value != 1500 {
				t.Fatalf("unexpected canonical reinforcement effect: %+v", effect)
			}
		}
	}
	if !foundReinforcement {
		t.Fatal("canonical reinforcement effect 483 was not emitted")
	}
	for _, extra := range stat.ExtraStats {
		if extra.EffectID == 483 {
			t.Fatalf("effect 483 should be compiled into AttackReinforcement, got extra stat %+v", extra)
		}
	}
}

func TestCommanderLegacyEffectDoesNotUseCastellanCatalogMapping(t *testing.T) {
	var stat CommStatModel

	ApplyCommanderLiveStat(&stat, 1, []float64{12}, CatalogEffectSourceEquipment)

	if stat.MeleeCbtStr != 12 {
		t.Fatalf("MeleeCbtStr = %v, want 12", stat.MeleeCbtStr)
	}
	if len(stat.Effects) != 0 {
		t.Fatalf("commander legacy effect resolved through castellan catalog: %+v", stat.Effects)
	}
}

func TestCommanderCatalogEffectIncludesCanonicalMetadata(t *testing.T) {
	var stat CommStatModel

	ApplyCommanderLiveStat(&stat, 175, []float64{25}, CatalogEffectSourceEquipment)

	if len(stat.Effects) != 1 {
		t.Fatalf("len(Effects) = %d, want 1", len(stat.Effects))
	}
	effect := stat.Effects[0]
	if effect.RawEffectID != 175 || effect.EffectID != 469 || effect.Scope != "pvp" || effect.Value != -25 {
		t.Fatalf("unexpected canonical effect: %+v", effect)
	}
	if effect.CapID != 2001 || effect.MaxTotalBonus == nil {
		t.Fatalf("canonical cap metadata missing: %+v", effect)
	}
}

func TestLiveEffectLabelUsesOfficialLangDescription(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{
			name: "newPVPAttackBonusUnitMead",
			want: "Attack strength for mead units when attacking against Castle Lords",
		},
		{
			name: "newPVPattackUnitAmountReinforcementBonus",
			want: "Mead ranged units for final assault against Castle Lords",
		},
		{
			name: "equipmentAREAttackUnitAmountFront",
			want: "Unit limit on the front in the RIft Raid Event",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := liveEffectLabel(liveEffectDefinition{Name: tc.name})
			if got != tc.want {
				t.Fatalf("liveEffectLabel(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRelicUnitToolEffectsUsePreviousCommanderBuckets(t *testing.T) {
	cases := []struct {
		name string
		id   float64
		want float64
		read func(CommStatModel) float64
	}{
		{name: "kingsguard", id: 20012, want: 12, read: func(s CommStatModel) float64 { return s.EliteStr }},
		{name: "horror", id: 20013, want: 13, read: func(s CommStatModel) float64 { return s.HorrorStr }},
		{name: "imperial", id: 20014, want: 14, read: func(s CommStatModel) float64 { return s.EliteStr }},
		{name: "beef", id: 20015, want: 15, read: func(s CommStatModel) float64 { return s.BeserkerStr }},
		{name: "relic barracks", id: 20016, want: 16, read: func(s CommStatModel) float64 { return s.RelicStr }},
		{name: "mead", id: 20017, want: 17, read: func(s CommStatModel) float64 { return s.MeadStr }},
		{name: "wave", id: 20018, want: 18, read: func(s CommStatModel) float64 { return s.Wave }},
		{name: "cooldown", id: 20019, want: 19, read: func(s CommStatModel) float64 { return s.Cooldown }},
		{name: "return travel", id: 20020, want: 20, read: func(s CommStatModel) float64 { return s.Travel }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stat CommStatModel

			ApplyCommanderLiveStat(&stat, tc.id, []float64{999, tc.want}, CatalogEffectSourceRelicEquipment)

			if got := tc.read(stat); got != tc.want {
				t.Fatalf("mapped value = %v, want %v", got, tc.want)
			}
			if len(stat.ExtraStats) != 0 {
				t.Fatalf("relic effect should not be exposed as extra stats, got %+v", stat.ExtraStats)
			}
			if len(stat.Effects) != 1 {
				t.Fatalf("canonical effect rows = %d, want 1", len(stat.Effects))
			}
		})
	}
}
