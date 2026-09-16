package GameData

import (
	"fmt"
	"sort"
	"strings"
)

// TimeSkipOption is an official inventory-backed minute skip. Duration is
// projected from currencyMinutesSkipValues rather than a feature-owned table.
type TimeSkipOption struct {
	CurrencyID int64
	WireKey    string
	Seconds    int64
}

func (store *Store) OfficialTimeSkips() ([]TimeSkipOption, error) {
	if store == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	currencies, err := store.Catalog("currencies")
	if err != nil {
		return nil, err
	}
	values, err := store.Catalog("currencyMinutesSkipValues")
	if err != nil {
		return nil, err
	}
	options := make([]TimeSkipOption, 0, 7)
	seen := map[string]struct{}{}
	for _, raw := range currencies.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		currencyID, hasID := record.Int64("currencyID")
		wireKey, hasKey := record.String("JSONKey")
		wireKey = strings.ToUpper(strings.TrimSpace(wireKey))
		if !hasID || currencyID <= 0 || !hasKey || !strings.HasPrefix(wireKey, "MS") {
			continue
		}
		rawValue, found := values.FindByField("currencyID", fmt.Sprintf("%d", currencyID))
		if !found {
			continue
		}
		value, decodeErr := DecodeRecord(rawValue)
		if decodeErr != nil {
			continue
		}
		minutes, valid := value.Int64("MinutesSkipValue")
		if !valid || minutes <= 0 || minutes > int64(^uint64(0)>>1)/60 {
			continue
		}
		if _, duplicate := seen[wireKey]; duplicate {
			return nil, fmt.Errorf("official time-skip key %s is duplicated", wireKey)
		}
		seen[wireKey] = struct{}{}
		options = append(options, TimeSkipOption{CurrencyID: currencyID, WireKey: wireKey, Seconds: minutes * 60})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("official time-skip catalog is unavailable")
	}
	sort.Slice(options, func(left, right int) bool {
		if options[left].Seconds != options[right].Seconds {
			return options[left].Seconds < options[right].Seconds
		}
		return options[left].WireKey < options[right].WireKey
	})
	return options, nil
}
