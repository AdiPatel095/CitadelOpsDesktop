package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const autoStationClientFileName = "AutoStation.json"

type AutoStationClientTroop struct {
	ID     int `json:"id"`
	Amount int `json:"amount"`
}

type AutoStationClientState struct {
	Version         int                                 `json:"version"`
	LeadTimeSec     int                                 `json:"leadTimeSec"`
	RecallWhenClear bool                                `json:"recallWhenClear"`
	MinRPTDays      int                                 `json:"minRPTDays"`
	Settings        map[string][]AutoStationClientTroop `json:"settings"`
}

var autoStationClientMu sync.Mutex

func defaultAutoStationClientState() AutoStationClientState {
	return AutoStationClientState{
		Version:         1,
		LeadTimeSec:     DefaultAutoStationLeadTimeSec,
		RecallWhenClear: true,
		MinRPTDays:      3,
		Settings:        make(map[string][]AutoStationClientTroop),
	}
}

func autoStationClientPath() string {
	return filepath.Join(Paths.DataDir(), autoStationClientFileName)
}

func normalizeAutoStationClientState(state AutoStationClientState) AutoStationClientState {
	config := AutoStationConfig{
		LeadTimeSec:     state.LeadTimeSec,
		RecallWhenClear: state.RecallWhenClear,
		MinRPTDays:      state.MinRPTDays,
		DefenseByCastle: make(map[int]map[int]int),
	}.Normalize()
	for castleKey, troops := range state.Settings {
		castleID, err := strconv.Atoi(castleKey)
		if err != nil || castleID <= 0 {
			continue
		}
		for _, troop := range troops {
			if troop.ID <= 0 || troop.Amount < 0 {
				continue
			}
			if config.DefenseByCastle[castleID] == nil {
				config.DefenseByCastle[castleID] = make(map[int]int)
			}
			config.DefenseByCastle[castleID][troop.ID] = troop.Amount
		}
	}
	return AutoStationClientStateFromConfig(config)
}

func AutoStationClientStateFromConfig(config AutoStationConfig) AutoStationClientState {
	config = config.Normalize()
	state := defaultAutoStationClientState()
	state.LeadTimeSec = config.LeadTimeSec
	state.RecallWhenClear = config.RecallWhenClear
	state.MinRPTDays = config.MinRPTDays
	for castleID, units := range config.DefenseByCastle {
		rows := make([]AutoStationClientTroop, 0, len(units))
		for unitID, amount := range units {
			rows = append(rows, AutoStationClientTroop{ID: unitID, Amount: amount})
		}
		state.Settings[strconv.Itoa(castleID)] = rows
	}
	return state
}

func (state AutoStationClientState) Config() AutoStationConfig {
	state = normalizeAutoStationClientState(state)
	config := AutoStationConfig{
		LeadTimeSec:     state.LeadTimeSec,
		RecallWhenClear: state.RecallWhenClear,
		MinRPTDays:      state.MinRPTDays,
		DefenseByCastle: make(map[int]map[int]int),
	}
	for castleKey, troops := range state.Settings {
		castleID, _ := strconv.Atoi(castleKey)
		config.DefenseByCastle[castleID] = make(map[int]int)
		for _, troop := range troops {
			config.DefenseByCastle[castleID][troop.ID] = troop.Amount
		}
	}
	return config.Normalize()
}

func ReadAutoStationClientFile() []byte {
	autoStationClientMu.Lock()
	defer autoStationClientMu.Unlock()
	data, err := os.ReadFile(autoStationClientPath())
	if err == nil && len(data) > 0 {
		var state AutoStationClientState
		if json.Unmarshal(data, &state) == nil {
			return marshalAutoStationClientState(normalizeAutoStationClientState(state))
		}
	}
	return marshalAutoStationClientState(defaultAutoStationClientState())
}

func ReadAutoStationConfig() AutoStationConfig {
	var state AutoStationClientState
	if json.Unmarshal(ReadAutoStationClientFile(), &state) != nil {
		return DefaultAutoStationConfig()
	}
	return state.Config()
}

func WriteAutoStationClientFile(data []byte) error {
	var state AutoStationClientState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	state = normalizeAutoStationClientState(state)
	data = marshalAutoStationClientState(state)
	autoStationClientMu.Lock()
	defer autoStationClientMu.Unlock()
	if err := os.MkdirAll(Paths.DataDir(), 0755); err != nil {
		return err
	}
	path := autoStationClientPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DefaultAutoStationClientJSON() []byte {
	return marshalAutoStationClientState(defaultAutoStationClientState())
}

func marshalAutoStationClientState(state AutoStationClientState) []byte {
	data, _ := json.MarshalIndent(state, "", "  ")
	return append(data, '\n')
}
