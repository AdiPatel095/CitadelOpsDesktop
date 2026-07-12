package AllianceTargets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestServiceLoadsTopAllianceTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/servers":
			_ = json.NewEncoder(writer).Encode([]string{"US1"})
		case "/alliances":
			page, _ := strconv.Atoi(request.URL.Query().Get("page"))
			rows := make([]map[string]string, 15)
			for index := range rows {
				gameID := int64(page*100 + index + 1)
				rows[index] = map[string]string{
					"alliance_id": fmt.Sprintf("%d000", gameID), "alliance_name": fmt.Sprintf("Alliance %d", gameID),
					"might_current": "5000", "player_count": "10",
				}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"alliances": rows})
		case "/alliances/id/101000":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"alliance_name": "Alliance 101",
				"players":       []map[string]string{{"player_id": "7000", "player_name": "Target", "might_current": "9000"}},
			})
		case "/cartography/id/101000":
			_ = json.NewEncoder(writer).Encode([]map[string]any{{"name": "Target", "castles": [][]int{{13, 14, 1}}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	service := NewService(server.Client())
	service.baseURL = server.URL
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "wss://ep-live-us1-game.example.test/socket"
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 10}

	view, err := service.View(t.Context(), gameState, nil, "", "101000", false)
	if err != nil {
		t.Fatal(err)
	}
	if view.Server != "US1" || len(view.Alliances) != 50 || view.SelectedAlliance == nil || view.SelectedAlliance.AllianceID != 101 {
		t.Fatalf("alliance view = %+v", view)
	}
	if len(view.Targets) != 1 || view.Targets[0].PlayerID != 7 || view.Targets[0].Distance != 5 {
		t.Fatalf("target view = %+v", view.Targets)
	}
}

func TestServerCandidatesResolveChromiumGameSocket(t *testing.T) {
	candidates := serverCandidates("wss://ep-live-us1-game.goodgamestudios.com/socket", []string{"DE1", "US1"})
	if len(candidates) != 1 || candidates[0] != "US1" {
		t.Fatalf("candidates = %#v", candidates)
	}
}
