package API

import (
	"testing"

	"CitadelDesktop/Server/GameData"
)

func TestApplyCraftingIconURLsUsesOfficialCurrencyAssets(t *testing.T) {
	catalog := GameData.CraftingCatalog{
		Resources: map[string]GameData.CraftingResource{
			"refinedLumber": {Key: "refinedLumber", AssetName: "RefinedLumber", IconURL: "/missing.webp"},
		},
		Recipes: []GameData.CraftingRecipe{{
			RecipeID: 6,
			Output:   GameData.CraftingRecipeOutput{Key: "refinedLumber", IconURL: "/missing.webp"},
		}},
	}

	applyCraftingIconURLs(&catalog, map[string]string{"RefinedLumber": "https://game.example/refined-lumber.webp"})

	if got := catalog.Resources["refinedLumber"].IconURL; got != "https://game.example/refined-lumber.webp" {
		t.Fatalf("resource icon URL = %q", got)
	}
	if got := catalog.Recipes[0].Output.IconURL; got != "https://game.example/refined-lumber.webp" {
		t.Fatalf("recipe icon URL = %q", got)
	}
}
