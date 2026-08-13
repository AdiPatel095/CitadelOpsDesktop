package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestPlayerTitlesTrackCurrentOwnerAndConnectionIndependently(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"titles":[
			{"titleID":"31","type":"FAME","displayType":"suffix"},
			{"titleID":"116","type":"FACTION","displayType":"prefix"},
			{"titleID":"117","type":"FACTION","displayType":"prefix"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	code := 0
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Session.ConnectionGeneration = 7
	frame := Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"M":[],"O":[
			{"OID":99,"PRE":116,"SUF":30,"TOPX":100,"CF":1},
			{"OID":42,"PRE":"116","SUF":"31","TOPX":"50","CF":"987654321"}
		]}`),
	}

	domains, changed, err := reducePlayerTitles(context.Background(), frame, &gameState, gameData)
	if err != nil || !changed || len(domains) != 3 || domains[0] != "player" ||
		domains[1] != "glory-title" || domains[2] != "gallantry-title" {
		t.Fatalf("reduce glory title domains=%v changed=%t err=%v", domains, changed, err)
	}
	if gameState.Player.GloryTitleID != 31 || gameState.Player.GloryTitleTopX != 50 ||
		gameState.Player.GloryTitleGen != 7 || !gameState.Player.GloryTitleAt.Equal(now) ||
		gameState.Player.Glory != 987654321 {
		t.Fatalf("glory title state=%#v", gameState.Player)
	}
	if gameState.Player.GallantryTitleID != 116 || gameState.Player.GallantryTitleGen != 7 ||
		!gameState.Player.GallantryTitleAt.Equal(now) {
		t.Fatalf("gallantry title state=%#v", gameState.Player)
	}
	if titleID, current := gameState.Player.CurrentGloryTitle(7); !current || titleID != 31 {
		t.Fatalf("current glory title=%d, %t", titleID, current)
	}
	if _, current := gameState.Player.CurrentGloryTitle(8); current {
		t.Fatal("title from an older connection was treated as current")
	}
	if titleID, current := gameState.Player.CurrentGallantryTitle(7); !current || titleID != 116 {
		t.Fatalf("current gallantry title=%d, %t", titleID, current)
	}
	if _, current := gameState.Player.CurrentGallantryTitle(8); current {
		t.Fatal("gallantry title from an older connection was treated as current")
	}

	metricFrame := Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(30 * time.Second),
		Payload: json.RawMessage(`{"M":[],"O":[{"OID":42,"PRE":116,"SUF":31,"TOPX":49,"CF":987654322}]}`),
	}
	domains, changed, err = reducePlayerTitles(context.Background(), metricFrame, &gameState, gameData)
	if err != nil || !changed || len(domains) != 1 || domains[0] != "player" {
		t.Fatalf("same-title metric update domains=%v changed=%t err=%v", domains, changed, err)
	}

	refreshFrame := metricFrame
	refreshFrame.ReceivedAt = now.Add(45 * time.Second)
	domains, changed, err = reducePlayerTitles(context.Background(), refreshFrame, &gameState, gameData)
	if err != nil || changed || len(domains) != 0 {
		t.Fatalf("same-title refresh domains=%v changed=%t err=%v", domains, changed, err)
	}

	gallantryFrame := metricFrame
	gallantryFrame.ReceivedAt = now.Add(50 * time.Second)
	gallantryFrame.Payload = json.RawMessage(`{"M":[],"O":[{"OID":42,"PRE":117,"SUF":31,"TOPX":49,"CF":987654322}]}`)
	domains, changed, err = reducePlayerTitles(context.Background(), gallantryFrame, &gameState, gameData)
	if err != nil || !changed || len(domains) != 2 || domains[0] != "player" || domains[1] != "gallantry-title" {
		t.Fatalf("gallantry-only title update domains=%v changed=%t err=%v", domains, changed, err)
	}
	if gameState.Player.GloryTitleID != 31 || gameState.Player.GallantryTitleID != 117 {
		t.Fatalf("gallantry title replaced glory title: %#v", gameState.Player)
	}

	partialFrame := Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Minute),
		Payload: json.RawMessage(`{"M":[],"O":[{"OID":42,"TOPX":1,"CF":2}]}`),
	}
	domains, changed, err = reducePlayerTitles(context.Background(), partialFrame, &gameState, gameData)
	if err != nil || changed || len(domains) != 0 {
		t.Fatalf("partial glory-title owner domains=%v changed=%t err=%v", domains, changed, err)
	}
	if gameState.Player.GloryTitleID != 31 || gameState.Player.GloryTitleTopX != 49 ||
		gameState.Player.Glory != 987654322 || !gameState.Player.GloryTitleAt.Equal(gallantryFrame.ReceivedAt) ||
		gameState.Player.GallantryTitleID != 117 || !gameState.Player.GallantryTitleAt.Equal(gallantryFrame.ReceivedAt) {
		t.Fatalf("partial owner overwrote confirmed glory-title state=%#v", gameState.Player)
	}
}
