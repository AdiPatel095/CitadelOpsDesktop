package GameData

import (
	"fmt"
	"sort"
)

type ConstructionShopProduct struct {
	PackageID          int64 `json:"packageId"`
	ConstructionItemID int64 `json:"constructionItemId"`
	Amount             int64 `json:"amount"`
}

func (store *Store) ConstructionShopProducts(itemID int64) ([]ConstructionShopProduct, error) {
	if store == nil || itemID <= 0 {
		return nil, fmt.Errorf("construction item id must be positive")
	}
	store.constructionShopOnce.Do(func() {
		store.constructionShopProducts, store.constructionShopErr = buildConstructionShopProducts(store)
	})
	if store.constructionShopErr != nil {
		return nil, store.constructionShopErr
	}
	return append([]ConstructionShopProduct(nil), store.constructionShopProducts[itemID]...), nil
}

func buildConstructionShopProducts(store *Store) (map[int64][]ConstructionShopProduct, error) {
	catalog, err := store.Catalog("packages")
	if err != nil {
		return nil, err
	}
	products := map[int64][]ConstructionShopProduct{}
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		packageType, _ := record.String("packageType")
		if packageType != "constructionItem" {
			continue
		}
		packageID, _ := record.Int64("packageID")
		itemID, _ := record.Int64("constructionItemID")
		amount, _ := record.Int64("constructionItemAmount")
		if packageID <= 0 || itemID <= 0 {
			continue
		}
		if amount <= 0 {
			amount = 1
		}
		products[itemID] = append(products[itemID], ConstructionShopProduct{
			PackageID: packageID, ConstructionItemID: itemID, Amount: amount,
		})
	}
	for itemID := range products {
		sort.Slice(products[itemID], func(left, right int) bool {
			return products[itemID][left].PackageID < products[itemID][right].PackageID
		})
	}
	return products, nil
}
