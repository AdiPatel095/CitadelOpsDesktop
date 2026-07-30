package History

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

func (store *Store) migrateLegacyPlayerTracker(sourcePath string) (int, error) {
	return store.ImportLegacySource(sourcePath, CollectionPlayerSamples, decodeLegacyPlayerTracker)
}

func decodeLegacyPlayerTracker(reader io.Reader, sink ImportSink) error {
	decoder := json.NewDecoder(reader)
	start, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("expected player tracker object")
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, _ := keyToken.(string)
		if key != "players" {
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return err
			}
			continue
		}
		playersStart, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := playersStart.(json.Delim); !ok || delimiter != '{' {
			return fmt.Errorf("expected players object")
		}
		for decoder.More() {
			playerToken, err := decoder.Token()
			if err != nil {
				return err
			}
			playerKey, _ := playerToken.(string)
			fallbackPlayerID, _ := strconv.ParseInt(playerKey, 10, 64)
			arrayStart, err := decoder.Token()
			if err != nil {
				return err
			}
			if delimiter, ok := arrayStart.(json.Delim); !ok || delimiter != '[' {
				return fmt.Errorf("expected samples array for player %s", playerKey)
			}
			for decoder.More() {
				var raw json.RawMessage
				if err := decoder.Decode(&raw); err != nil {
					return err
				}
				var header struct {
					TimestampUnix int64 `json:"timestampUnix"`
					PlayerID      int64 `json:"playerId"`
				}
				if json.Unmarshal(raw, &header) != nil || header.TimestampUnix <= 0 {
					continue
				}
				if header.PlayerID == 0 && fallbackPlayerID > 0 {
					var document map[string]any
					if json.Unmarshal(raw, &document) == nil {
						document["playerId"] = fallbackPlayerID
						raw, _ = json.Marshal(document)
					}
				}
				if err := sink(ImportRecord{
					CapturedAt: time.Unix(header.TimestampUnix, 0).UTC(), Payload: raw,
				}); err != nil {
					return err
				}
			}
			if _, err := decoder.Token(); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
