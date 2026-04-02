package craftingrecipes

import "CitadelDesktop/Server/Models/Castle"

// EnrichSlotBundle adds parallel `labels` for each CRID for the frontend.
func EnrichSlotBundle(b castle.CraftingSlotBundle) map[string]interface{} {
	labels := make([]string, len(b.CRID))
	for i, id := range b.CRID {
		labels[i] = ShortLabel(id)
	}
	out := map[string]interface{}{
		"crid":   b.CRID,
		"labels": labels,
	}
	if len(b.BV) > 0 {
		out["bv"] = b.BV
	}
	return out
}

// EnrichCraftingQueues builds JSON-safe maps for castleFocus (per-building crafting from **crin** / **crst**).
func EnrichCraftingQueues(q []castle.CraftingBuildingSnapshot) []map[string]interface{} {
	if len(q) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(q))
	for _, s := range q {
		out = append(out, map[string]interface{}{
			"kid":  s.KID,
			"aid":  s.AID,
			"oid":  s.OID,
			"wid":  s.WID,
			"cqid": s.CQID,
			"ps":   EnrichSlotBundle(s.PS),
			"qs":   EnrichSlotBundle(s.QS),
		})
	}
	return out
}
