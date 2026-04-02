// Package craftingrecipes loads EmpireItems/craftingRecipes.json for CRID → recipeGroupID / queueTypeId labels.
package craftingrecipes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"CitadelDesktop/Server/Data/EmpireItems"
)

// Meta is a subset of one crafting recipe row.
type Meta struct {
	CRID         int
	RecipeGroup  int
	QueueType    int
	RewardIDs    string
	CraftingSecs int
}

var (
	loadOnce sync.Once
	byCRID   map[int]Meta
	loadErr  error
)

func ensureLoaded() {
	loadOnce.Do(func() {
		var rows []struct {
			CraftingRecipeId string `json:"craftingRecipeId"`
			RecipeGroupID    string `json:"recipeGroupID"`
			QueueTypeId      string `json:"queueTypeId"`
			RewardIDs        string `json:"rewardIDs"`
			CraftingDuration string `json:"craftingDuration"`
		}
		if err := json.Unmarshal(empireitems.CraftingRecipesJSON, &rows); err != nil {
			loadErr = err
			byCRID = map[int]Meta{}
			return
		}
		m := make(map[int]Meta, len(rows))
		for _, r := range rows {
			crid, e1 := strconv.Atoi(r.CraftingRecipeId)
			if e1 != nil || crid <= 0 {
				continue
			}
			g, _ := strconv.Atoi(r.RecipeGroupID)
			q, _ := strconv.Atoi(r.QueueTypeId)
			dur, _ := strconv.Atoi(r.CraftingDuration)
			m[crid] = Meta{
				CRID: crid, RecipeGroup: g, QueueType: q,
				RewardIDs: r.RewardIDs, CraftingSecs: dur,
			}
		}
		byCRID = m
	})
}

// MetaForCRID returns recipe metadata from craftingRecipes.json, if present.
func MetaForCRID(crid int) (Meta, bool) {
	ensureLoaded()
	if loadErr != nil {
		return Meta{}, false
	}
	meta, ok := byCRID[crid]
	return meta, ok
}

// ShortLabel is a compact dashboard label (queueType groups map to the four manual buildings).
func ShortLabel(crid int) string {
	meta, ok := MetaForCRID(crid)
	if !ok {
		return fmt.Sprintf("CRID %d", crid)
	}
	return fmt.Sprintf("CRID %d · Q%d · G%d", crid, meta.QueueType, meta.RecipeGroup)
}
