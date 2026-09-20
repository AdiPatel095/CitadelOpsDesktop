package API

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestEquipmentOptimizeHTTPReturnsBoundedAlternatives(t *testing.T) {
	handler, body := representativeEquipmentOptimizeFixture(t, "commander", 334, 16)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/equipment/optimize", bytes.NewReader(body))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response Equipment.OptimizeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Alternatives) != 10 {
		t.Fatalf("alternatives = %d, want 10", len(response.Alternatives))
	}
	if response.SnapshotFingerprint == "" {
		t.Fatal("response omitted snapshot fingerprint")
	}
	if fmt.Sprint(response.Proposed.Equipment, response.Proposed.Gems) != fmt.Sprint(response.Alternatives[0].Equipment, response.Alternatives[0].Gems) {
		t.Fatal("legacy proposed loadout differs from first alternative")
	}
}

func TestEquipmentOptimizeRepresentativeHTTPPerformance(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		kind      string
		equipment int
		gems      int
	}{
		{name: "commander-334-equipment-16-gems", kind: "commander", equipment: 334, gems: 16},
		{name: "castellan-253-equipment-no-pvp-gems", kind: "castellan", equipment: 253, gems: 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			handler, body := representativeEquipmentOptimizeFixture(t, fixture.kind, fixture.equipment, fixture.gems)
			for warmup := 0; warmup < 2; warmup++ {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v2/equipment/optimize", bytes.NewReader(body)))
				if recorder.Code != http.StatusOK {
					t.Fatalf("warmup status = %d: %s", recorder.Code, recorder.Body.String())
				}
			}
			durations := make([]time.Duration, 12)
			for index := range durations {
				started := time.Now()
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v2/equipment/optimize", bytes.NewReader(body)))
				durations[index] = time.Since(started)
				if recorder.Code != http.StatusOK {
					t.Fatalf("measured status = %d: %s", recorder.Code, recorder.Body.String())
				}
			}
			sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
			p95 := durations[len(durations)*95/100]
			t.Logf("warm HTTP handler request + optimize + JSON p95=%s samples=%d", p95, len(durations))
			if p95 >= time.Second {
				t.Fatalf("warm HTTP p95 = %s, want <1s", p95)
			}
		})
	}
}

func representativeEquipmentOptimizeFixture(t testing.TB, kind string, equipmentCount int, gemCount int) (http.Handler, []byte) {
	t.Helper()
	cacheDir := t.TempDir()
	catalog := `{"versionInfo":{},"buildings":[],"units":[],"effectCaps":[{"capID":"23","maxTotalBonus":"90"}],"effects":[{"effectID":"9001","capID":"23"},{"effectID":"9002","capID":"23"},{"effectID":"9003","capID":"23"},{"effectID":"9004"},{"effectID":"9005"},{"effectID":"9006"},{"effectID":"9007"},{"effectID":"9008"}]}`
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vfixture.json"), []byte(catalog), 0o600); err != nil {
		t.Fatal(err)
	}
	gameData := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cacheDir})
	if err := gameData.LoadCache(); err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	equipmentType := 2
	if kind == "castellan" {
		equipmentType = 1
	}
	currentEquipment := map[string]State.EquipmentInstanceID{}
	slots := []int{1, 2, 3, 4, 6}
	for index := 0; index < equipmentCount; index++ {
		slot := slots[index%len(slots)]
		id := State.EquipmentInstanceID(1000 + index)
		effects := State.EquipmentEffects{
			{WireID: int64(index%8 + 1), DefinitionID: int64(9001 + index%8), Values: []float64{float64(index%37 + 1)}},
			{WireID: int64((index+3)%8 + 1), DefinitionID: int64(9001 + (index+3)%8), Values: []float64{float64(index%19 + 1)}},
		}
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, DefinitionID: State.EquipmentID(3000 + index), Slot: slot, TypeID: equipmentType, RelicKnown: true, SetID: int64(index % 12), Effects: effects}
		if index < len(slots) {
			currentEquipment[fmt.Sprint(slot)] = id
		}
	}
	currentGems := map[string]State.GemInstanceID{}
	for index := 0; index < gemCount; index++ {
		id := -State.GemInstanceID(5000 + index)
		gameState.Inventory.Gems[id] = State.GemInstance{ID: id, DefinitionID: State.GemID(7000 + index), CompatibleWearerID: equipmentType, CombatMode: "pvp", Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: int64(9001 + index%8), Values: []float64{float64(index%13 + 1)}}}}
		if index < 4 {
			currentGems[fmt.Sprint(index+1)] = id
		}
	}
	if kind == "commander" {
		gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: currentEquipment, Gems: currentGems}
	} else {
		gameState.Castellans[0] = State.CastellanState{ID: 0, Equipment: currentEquipment, Gems: currentGems}
	}
	priorities := make([]Equipment.Priority, 8)
	for index := range priorities {
		priorities[index] = Equipment.Priority{EffectID: int64(9001 + index), Tier: 1 + index/4, Position: index / 2}
	}
	body, err := json.Marshal(Equipment.OptimizeRequest{LeaderKind: kind, LeaderID: 0, CombatMode: "pvp", Priorities: priorities, ResultCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(Config{State: State.NewStore(gameState), GameData: gameData}).Handler(), body
}
