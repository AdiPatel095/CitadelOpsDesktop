package GameData

import "testing"

func TestConstructionShopProductsComeFromOfficialPackages(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"packages":[
			{"packageID":20,"packageType":"currency","constructionItemID":101},
			{"packageID":11,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":2},
			{"packageID":10,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":1}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	products, err := store.ConstructionShopProducts(101)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 2 || products[0].PackageID != 10 || products[1].PackageID != 11 || products[1].Amount != 2 {
		t.Fatalf("unexpected products: %+v", products)
	}
}
