package GameParser

import (
	"encoding/json"
	"fmt"
)

// GbcTrivialCIPLRow is one **gbc** product line (PID, AMT) from EmpireEx_21 field PL
// (copied into Tci session storage with the same numbers).
type GbcTrivialCIPLRow struct {
	PID int
	AMT int
}

// ParseGbcTrivialCIPLFromJSON extracts PL from a **gbc** JSON body. Empty PL yields an empty slice.
func ParseGbcTrivialCIPLFromJSON(payload string) ([]GbcTrivialCIPLRow, error) {
	var root struct {
		PL []struct {
			PID float64 `json:"PID"`
			AMT float64 `json:"AMT"`
		} `json:"PL"`
	}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return nil, fmt.Errorf("gbc PL: %w", err)
	}
	if len(root.PL) == 0 {
		return nil, nil
	}
	rows := make([]GbcTrivialCIPLRow, 0, len(root.PL))
	for _, r := range root.PL {
		rows = append(rows, GbcTrivialCIPLRow{PID: int(r.PID), AMT: int(r.AMT)})
	}
	return rows, nil
}
