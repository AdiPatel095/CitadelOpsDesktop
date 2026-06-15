package GameParser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// TrivialCIPurchaseInfo maps a constructionItemID to **gbc** / **sbp** row (PID, AMT). JSON key is
// constructionItemID string. Example: { "30130": { "pid": 2493, "amt": 1 } }. Amt 0 is treated as 1 when sent.
// Shipped map is built from Server/Data/packages/items.json (Central Silver Shop constructionItem rows).
// Optional DataDir CidTrivialProduct.json overrides or extends by wire CID.
type TrivialCIPurchaseInfo struct {
	PID int `json:"pid"`
	// Amt is optional; 0 is normalized to 1 at load / send time.
	Amt int `json:"amt"`
}

var (
	cidTrivialProductOnce  sync.Once
	cidTrivialProductByCid map[int]TrivialCIPurchaseInfo
	cidTrivialProductErr   error
)

const cidTrivialProductFile = "CidTrivialProduct.json"

// findCidTrivialProductPath returns the optional on-disk path for per-user / per-instance
// overrides: Paths.DataDir / CidTrivialProduct.json (not under Server/Data).
// When absent, only packages/items.json applies.
func findCidTrivialProductPath() string {
	if p := filepath.Join(Paths.DataDir(), cidTrivialProductFile); fileExists(p) {
		return p
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func buildCidTrivialProduct() {
	m, err := loadTrivialCIPurchaseFromPackages()
	if err != nil {
		cidTrivialProductErr = err
		cidTrivialProductByCid = map[int]TrivialCIPurchaseInfo{}
	} else {
		cidTrivialProductByCid = m
	}
	p := findCidTrivialProductPath()
	if p == "" {
		cidTrivialProductErr = nil
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		cidTrivialProductErr = err
		return
	}
	var raw map[string]TrivialCIPurchaseInfo
	if err := json.Unmarshal(b, &raw); err != nil {
		cidTrivialProductErr = err
		return
	}
	cidTrivialProductErr = nil
	for k, v := range raw {
		id, err := strconv.Atoi(k)
		if err != nil || id <= 0 {
			continue
		}
		if v.Amt < 0 {
			v.Amt = 0
		}
		if v.Amt == 0 {
			v.Amt = 1
		}
		if v.PID > 0 {
			cidTrivialProductByCid[id] = v
		}
	}
}

// TrivialCIPurchaseInfoForCid returns shop PID+AMT for a constructionItemID if listed in the merged map
// (packages/items.json plus optional CidTrivialProduct.json overrides). ok is false if unmapped.
func TrivialCIPurchaseInfoForCid(wireCid int) (info TrivialCIPurchaseInfo, ok bool) {
	if wireCid <= 0 {
		return TrivialCIPurchaseInfo{}, false
	}
	cidTrivialProductOnce.Do(buildCidTrivialProduct)
	if cidTrivialProductByCid == nil {
		return TrivialCIPurchaseInfo{}, false
	}
	n, o := cidTrivialProductByCid[wireCid]
	return n, o
}
