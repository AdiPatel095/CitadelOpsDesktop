// Command cit6-equipment-fixture serves isolated synthetic equipment states through
// the production API mux. It never opens a game session or sends game commands.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const catalogJSON = `{
  "versionInfo": {}, "buildings": [], "units": [],
  "effectCaps": [{"capID":"23","maxTotalBonus":"90"}],
  "effecttypes": [
    {"effectTypeID":"101","name":"MeleeCombatStrength","sortCategory":"3","sortGroup":"1"},
    {"effectTypeID":"102","name":"MeleeWallDamage","sortCategory":"3","sortGroup":"1"},
    {"effectTypeID":"103","name":"RangedCombatStrength","sortCategory":"3","sortGroup":"2"},
    {"effectTypeID":"104","name":"RangedWallDamage","sortCategory":"3","sortGroup":"2"},
    {"effectTypeID":"105","name":"CourtyardCombatStrength","sortCategory":"5","sortGroup":"3"},
    {"effectTypeID":"106","name":"UnitAmountYard","sortCategory":"5","sortGroup":"3"},
    {"effectTypeID":"107","name":"WallProtectionReduction","sortCategory":"5","sortGroup":"4"},
    {"effectTypeID":"108","name":"GateProtectionReduction","sortCategory":"5","sortGroup":"4"},
    {"effectTypeID":"111","name":"MeleeCombatStrength","sortCategory":"3","sortGroup":"1"},
    {"effectTypeID":"112","name":"MeleeCombatStrength","sortCategory":"3","sortGroup":"1"}
  ],
  "effects": [
    {"effectID":"9001","effectTypeID":"101","capID":"23"}, {"effectID":"9002","effectTypeID":"102","capID":"23"},
    {"effectID":"9003","effectTypeID":"103","capID":"23"}, {"effectID":"9004","effectTypeID":"104"},
    {"effectID":"9005","effectTypeID":"105"}, {"effectID":"9006","effectTypeID":"106"},
    {"effectID":"9007","effectTypeID":"107"}, {"effectID":"9008","effectTypeID":"108"},
    {"effectID":"9011","effectTypeID":"111"}, {"effectID":"9012","effectTypeID":"112"}
  ]
}`

type fixtureHandlers struct {
	normal map[string]http.Handler
	few    map[string]http.Handler
	empty  map[string]http.Handler
}

func main() {
	listen := flag.String("listen", "127.0.0.1:41732", "loopback address for the fixture API")
	flag.Parse()
	if !isLoopbackAddress(*listen) {
		log.Fatalf("fixture refuses non-loopback listen address %q", *listen)
	}

	cacheDir, err := os.MkdirTemp("", "cit6-equipment-fixture-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vfixture.json"), []byte(catalogJSON), 0o600); err != nil {
		log.Fatal(err)
	}
	gameData := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cacheDir})
	if err := gameData.LoadCache(); err != nil {
		log.Fatal(err)
	}

	handlers := fixtureHandlers{
		normal: map[string]http.Handler{
			"commander": productionHandler(gameData, syntheticState("commander", 334, 16)),
			"castellan": productionHandler(gameData, syntheticState("castellan", 253, 0)),
		},
		few: map[string]http.Handler{
			"commander": productionHandler(gameData, syntheticState("commander", 4, 0)),
			"castellan": productionHandler(gameData, syntheticState("castellan", 4, 0)),
		},
		empty: map[string]http.Handler{
			"commander": productionHandler(gameData, syntheticState("commander", 0, 0)),
			"castellan": productionHandler(gameData, syntheticState("castellan", 0, 0)),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /fixture/health", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]any{
			"ready":     true,
			"synthetic": true,
			"solver":    "Server/Equipment.Optimize",
			"route":     "/api/v2/equipment/optimize",
		})
	})
	mux.Handle("POST /api/v2/equipment/optimize", handlers)

	server := &http.Server{
		Addr:              *listen,
		Handler:           requestLog(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("CIT-7 synthetic fixture API on http://%s", *listen)
	log.Fatal(server.ListenAndServe())
}

func (handlers fixtureHandlers) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 1<<20))
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	var input Equipment.OptimizeRequest
	if err := json.Unmarshal(body, &input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	kind := strings.ToLower(strings.TrimSpace(input.LeaderKind))
	if kind != "commander" && kind != "castellan" {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "leaderKind must be commander or castellan"}})
		return
	}

	scenario := request.URL.Query().Get("scenario")
	if scenario == "error" {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"code": "fixture_error", "message": "Synthetic optimizer service failure"}})
		return
	}
	if scenario == "timeout" {
		timer := time.NewTimer(9 * time.Second)
		defer timer.Stop()
		select {
		case <-request.Context().Done():
			return
		case <-timer.C:
		}
	} else if scenario == "delayed" {
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-request.Context().Done():
			return
		case <-timer.C:
		}
	}

	target := handlers.normal[kind]
	switch scenario {
	case "few":
		target = handlers.few[kind]
	case "no-gear":
		target = handlers.empty[kind]
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	writer.Header().Set("X-CIT6-Fixture", "synthetic")
	writer.Header().Set("X-CIT6-Solver", "production-api-mux")
	target.ServeHTTP(writer, request)
}

func productionHandler(gameData *GameData.Manager, state State.GameState) http.Handler {
	return API.NewServer(API.Config{State: State.NewStore(state), GameData: gameData}).Handler()
}

func syntheticState(kind string, equipmentCount, gemCount int) State.GameState {
	state := State.NewGameState()
	state.Revision = 41
	state.CatalogVersion = "fixture"
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Account.WorldID = "cit6-fixture"
	state.Account.PlayerID = 77
	state.Player.ID = 77

	equipmentType := 2
	baseID := 1000
	if kind == "castellan" {
		equipmentType = 1
		baseID = 2000
	}
	currentEquipment := map[string]State.EquipmentInstanceID{}
	slots := []int{1, 2, 3, 4, 6}
	for index := 0; index < equipmentCount; index++ {
		slot := slots[index%len(slots)]
		id := State.EquipmentInstanceID(baseID + index)
		state.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, DefinitionID: State.EquipmentID(3000 + baseID + index),
			Slot: slot, TypeID: equipmentType, RarityID: index % 6, SetID: int64(index % 12), Level: index % 21, RelicKnown: true,
			Effects: State.EquipmentEffects{
				{WireID: int64(index%8 + 1), DefinitionID: int64(9001 + index%8), Values: []float64{float64(index%37 + 1)}},
				{WireID: int64((index+3)%8 + 1), DefinitionID: int64(9001 + (index+3)%8), Values: []float64{float64(index%19 + 1)}},
			},
		}
		if _, exists := currentEquipment[fmt.Sprint(slot)]; !exists {
			currentEquipment[fmt.Sprint(slot)] = id
		}
	}
	currentGems := map[string]State.GemInstanceID{}
	for index := 0; index < gemCount; index++ {
		id := State.GemInstanceID(5000 + index)
		state.Inventory.Gems[id] = State.GemInstance{
			ID: id, DefinitionID: State.GemID(7000 + index), CompatibleWearerID: equipmentType,
			CombatMode: "pvp", Level: index % 16,
			Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: int64(9001 + index%8), Values: []float64{float64(index%13 + 1)}}},
		}
		if index < 4 {
			currentGems[fmt.Sprint(index+1)] = id
		}
	}
	if kind == "commander" {
		state.Commanders[0] = State.CommanderState{ID: 0, Name: "Fixture Commander", Available: true, Equipment: currentEquipment, Gems: currentGems}
	} else {
		state.Castellans[0] = State.CastellanState{ID: 0, Name: "Fixture Castellan", Equipment: currentEquipment, Gems: currentGems}
	}
	return state
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		next.ServeHTTP(writer, request)
		log.Printf("%s %s scenario=%s duration=%s", request.Method, request.URL.Path, request.URL.Query().Get("scenario"), time.Since(started))
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func isLoopbackAddress(address string) bool {
	host := address
	if index := strings.LastIndex(address, ":"); index >= 0 {
		host = address[:index]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
