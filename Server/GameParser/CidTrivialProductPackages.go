package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"

	serverdata "CitadelDesktop/Server/Data"
)

// centralSilverShopMarker tags live trivial-shop construction rows in packages/items.json.
const centralSilverShopMarker = "Central Silver Shop - keep like this"

type packageCatalogRow struct {
	PackageID              string `json:"packageID"`
	PackageType            string `json:"packageType"`
	Comment2               string `json:"comment2"`
	ConstructionItemID     string `json:"constructionItemID"`
	ConstructionItemAmount string `json:"constructionItemAmount"`
}

// loadTrivialCIPurchaseFromPackages builds CID→{PID,AMT} from official packages/items.json
// (packageType constructionItem + Central Silver Shop marker). Matches in-game **gbc**/**sbp** rows.
func loadTrivialCIPurchaseFromPackages() (map[int]TrivialCIPurchaseInfo, error) {
	b, err := serverdata.ReadPackagesItemsJSON()
	if err != nil {
		return nil, err
	}
	var rows []packageCatalogRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := make(map[int]TrivialCIPurchaseInfo)
	for _, row := range rows {
		if row.PackageType != "constructionItem" {
			continue
		}
		if !strings.Contains(row.Comment2, centralSilverShopMarker) {
			continue
		}
		cid, err := strconv.Atoi(strings.TrimSpace(row.ConstructionItemID))
		if err != nil || cid <= 0 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(row.PackageID))
		if err != nil || pid <= 0 {
			continue
		}
		amt := 1
		if row.ConstructionItemAmount != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(row.ConstructionItemAmount)); err == nil && n > 0 {
				amt = n
			}
		}
		out[cid] = TrivialCIPurchaseInfo{PID: pid, Amt: amt}
	}
	return out, nil
}
