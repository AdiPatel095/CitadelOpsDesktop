package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
	"time"
)

// SDIMessage is the subset we care about from %xt%sdi% responses.
// Example (truncated):
// {"SCID":16339029,"gaa":{"AI":[...]}, ...}
type SDIMessage struct {
	SCID int `json:"SCID"`
	GAA  struct {
		AI []any `json:"AI"`
	} `json:"gaa"`
}

// ParseSDIMessage captures the SDI route context used by CDS.
//
// Empirically, gaa.AI[17] contains the LID value that must be echoed in CDS.
func ParseSDIMessage(payload string) {
	var msg SDIMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[parser] sdi unmarshal: %v", err)
		return
	}
	if msg.SCID <= 0 {
		return
	}

	lid := 0
	if len(msg.GAA.AI) > 17 {
		switch v := msg.GAA.AI[17].(type) {
		case float64:
			lid = int(v)
		case int:
			lid = v
		}
	}

	gs := Models.GetGameState()
	gs.SetLastSDI(msg.SCID, lid, time.Now().UnixNano())
}
