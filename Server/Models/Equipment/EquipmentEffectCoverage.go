package equipment

import (
	"slices"
	"sort"
)

// EffectCoverageReport lists effect/stat IDs present in live websocket-style payloads that have no
// matching live-row handler or catalog-backed resolver. Use GameParser.RawEquipmentEffectCoverageFromGLI
// (and related helpers there) to read IDs from raw GLI/storage/RGEM maps at the same indices as
// ProcessEquipment / ProcessGem—before ProcessEquipStat* drops unknown IDs.
type EffectCoverageReport struct {
	// CommMissing — seen on commander loadout or comm-type storage (equip type 2) but not in CommStatUpdaterMap.
	CommMissing []float64
	// CastMissing — seen on castellan loadout or cast-type storage (equip type 1) but not in CastStatUpdaterMap.
	CastMissing []float64
	// GemStashMissing — relic gems in RGEM with a stat ID missing from both maps.
	GemStashMissing []float64
}

func sortedKeys(m map[float64]struct{}) []float64 {
	if len(m) == 0 {
		return nil
	}
	out := make([]float64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Float64s(out)
	return out
}

// EffectCoverageReportFromSets builds a report from deduplication sets (nil maps are treated as empty).
func EffectCoverageReportFromSets(commMissing, castMissing, gemStashMissing map[float64]struct{}) EffectCoverageReport {
	return EffectCoverageReport{
		CommMissing:     sortedKeys(commMissing),
		CastMissing:     sortedKeys(castMissing),
		GemStashMissing: sortedKeys(gemStashMissing),
	}
}

// EffectCoverageOK is true when there are no missing effect handlers.
func (r EffectCoverageReport) EffectCoverageOK() bool {
	return len(r.CommMissing) == 0 && len(r.CastMissing) == 0 && len(r.GemStashMissing) == 0
}

// Merge combines reports (e.g. from repeated snapshots); slices are de-duplicated and sorted.
func (r EffectCoverageReport) Merge(o EffectCoverageReport) EffectCoverageReport {
	set := func(a []float64) map[float64]struct{} {
		m := make(map[float64]struct{})
		for _, x := range a {
			m[x] = struct{}{}
		}
		return m
	}
	cm := set(r.CommMissing)
	for _, x := range o.CommMissing {
		cm[x] = struct{}{}
	}
	ca := set(r.CastMissing)
	for _, x := range o.CastMissing {
		ca[x] = struct{}{}
	}
	gs := set(r.GemStashMissing)
	for _, x := range o.GemStashMissing {
		gs[x] = struct{}{}
	}
	return EffectCoverageReport{
		CommMissing:     sortedKeys(cm),
		CastMissing:     sortedKeys(ca),
		GemStashMissing: sortedKeys(gs),
	}
}

// CommStatUpdaterKeys returns sorted keys of CommStatUpdaterMap (for tests / diagnostics).
func CommStatUpdaterKeys() []float64 {
	out := make([]float64, 0, len(CommStatUpdaterMap))
	for k := range CommStatUpdaterMap {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// CastStatUpdaterKeys returns sorted keys of CastStatUpdaterMap.
func CastStatUpdaterKeys() []float64 {
	out := make([]float64, 0, len(CastStatUpdaterMap))
	for k := range CastStatUpdaterMap {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
