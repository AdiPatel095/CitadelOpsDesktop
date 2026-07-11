package featureview

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	spyreport "CitadelDesktop/Server/Models/SpyReport"
	"CitadelDesktop/Server/PlayerTracker"
	"CitadelDesktop/Server/ResponseRegistry"
)

const allianceTargetAPI = "https://api.gge-tracker.com/api/v1"

var allianceTargetHTTPClient = &http.Client{Timeout: 10 * time.Second}

type trackerInt int64

func (n *trackerInt) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), `"`)
	if raw == "" || raw == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*n = trackerInt(v)
	return nil
}

type AllianceTargetAlliance struct {
	ExternalID  string `json:"externalId"`
	AllianceID  int    `json:"allianceId"`
	Name        string `json:"name"`
	Rank        int    `json:"rank"`
	Might       int64  `json:"might"`
	PlayerCount int    `json:"playerCount"`
}

type AllianceTargetCastle struct {
	CastleID int    `json:"castleId,omitempty"`
	Name     string `json:"name"`
	TypeName string `json:"typeName,omitempty"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Type     int    `json:"type,omitempty"`
}

type AllianceTargetPlayer struct {
	PlayerID         int                  `json:"playerId"`
	Name             string               `json:"name"`
	Might            int64                `json:"might"`
	UnderBird        bool                 `json:"underBird"`
	RPTSeconds       int                  `json:"rptSeconds"`
	BirdUntil        string               `json:"birdUntil,omitempty"`
	UpdatedAt        string               `json:"updatedAt,omitempty"`
	TargetCastle     AllianceTargetCastle `json:"targetCastle"`
	ClosestOwnCastle AllianceTargetCastle `json:"closestOwnCastle"`
	Distance         float64              `json:"distance"`
}

type SpyTavern struct {
	Level    int `json:"level"`
	Capacity int `json:"capacity"`
}

type SpyAvailability struct {
	Capacity           int                  `json:"capacity"`
	Active             int                  `json:"active"`
	Available          int                  `json:"available"`
	BuildingRowsLoaded bool                 `json:"buildingRowsLoaded"`
	SourceCastle       AllianceTargetCastle `json:"sourceCastle"`
	Taverns            []SpyTavern          `json:"taverns"`
}

type AllianceTargetView struct {
	Server           string                   `json:"server"`
	Alliances        []AllianceTargetAlliance `json:"alliances"`
	SelectedAlliance *AllianceTargetAlliance  `json:"selectedAlliance,omitempty"`
	Targets          []AllianceTargetPlayer   `json:"targets"`
	Spies            SpyAvailability          `json:"spies"`
	FetchedAt        string                   `json:"fetchedAt"`
}

type trackerAlliancePage struct {
	Alliances []struct {
		AllianceID  string     `json:"alliance_id"`
		Name        string     `json:"alliance_name"`
		Might       trackerInt `json:"might_current"`
		PlayerCount trackerInt `json:"player_count"`
	} `json:"alliances"`
}

type trackerAllianceDetail struct {
	Name    string `json:"alliance_name"`
	Players []struct {
		PlayerID  string     `json:"player_id"`
		Name      string     `json:"player_name"`
		Might     trackerInt `json:"might_current"`
		BirdUntil string     `json:"peace_disabled_at"`
		UpdatedAt string     `json:"updated_at"`
	} `json:"players"`
}

type trackerCartographyPlayer struct {
	Name    string  `json:"name"`
	Castles [][]int `json:"castles"`
}

type liveAllianceRoster struct {
	Alliance struct {
		AllianceID int                  `json:"AID"`
		Name       string               `json:"N"`
		Members    []liveAllianceMember `json:"M"`
	} `json:"A"`
}

type liveAllianceMember struct {
	PlayerID int     `json:"OID"`
	Name     string  `json:"N"`
	RPT      int     `json:"RPT"`
	Castles  [][]int `json:"AP"`
}

var allianceListCache struct {
	sync.Mutex
	Server    string
	FetchedAt time.Time
	Rows      []AllianceTargetAlliance
}

var tavernCapacityByWID = map[int]SpyTavern{
	145:  {Level: 1, Capacity: 2},
	146:  {Level: 2, Capacity: 3},
	147:  {Level: 3, Capacity: 5},
	533:  {Level: 4, Capacity: 7},
	771:  {Level: 5, Capacity: 9},
	772:  {Level: 6, Capacity: 11},
	773:  {Level: 7, Capacity: 13},
	757:  {Level: 8, Capacity: 16},
	758:  {Level: 9, Capacity: 19},
	2988: {Level: 10, Capacity: 21},
}

func LoadAllianceTargetView(ctx context.Context, selectedExternalID string) (AllianceTargetView, error) {
	server, err := playertracker.ActiveGGETrackerServer(ctx)
	if err != nil {
		return AllianceTargetView{}, err
	}
	alliances, err := loadTopAlliances(ctx, server)
	if err != nil {
		return AllianceTargetView{}, err
	}
	view := AllianceTargetView{
		Server:    server,
		Alliances: alliances,
		Spies:     currentSpyAvailability(),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if selectedExternalID == "" {
		currentAllianceID := Models.GetGameState().Alliance.AID
		for _, alliance := range alliances {
			if alliance.AllianceID > 0 && alliance.AllianceID != currentAllianceID {
				selectedExternalID = alliance.ExternalID
				break
			}
		}
		if selectedExternalID == "" {
			return view, nil
		}
	}
	var selected *AllianceTargetAlliance
	for i := range alliances {
		if alliances[i].ExternalID == selectedExternalID {
			copy := alliances[i]
			selected = &copy
			break
		}
	}
	if selected == nil {
		return AllianceTargetView{}, fmt.Errorf("selected alliance is not in the current top 50")
	}
	detail, cartography, err := loadAllianceTargets(ctx, server, selectedExternalID)
	if err != nil {
		return AllianceTargetView{}, err
	}
	if detail.Name != "" {
		selected.Name = detail.Name
	}
	view.SelectedAlliance = selected
	if live, liveErr := loadLiveAllianceRoster(selected.AllianceID); liveErr == nil && live.Alliance.AllianceID == selected.AllianceID {
		if live.Alliance.Name != "" {
			selected.Name = live.Alliance.Name
		}
		view.Targets = buildLiveAllianceTargets(live, detail)
	} else {
		view.Targets = buildAllianceTargets(detail, cartography)
	}
	enrichTargetCastleNames(view.Targets)
	return view, nil
}

func loadLiveAllianceRoster(allianceID int) (liveAllianceRoster, error) {
	var roster liveAllianceRoster
	if allianceID <= 0 || !ResponseRegistry.IsGameWebSocketReady() {
		return roster, fmt.Errorf("live alliance roster is unavailable")
	}
	waiter := ResponseRegistry.Global.RegisterWaiterMatching("ain", 5*time.Second, matchAINAlliance(allianceID), nil)
	defer waiter.Cleanup()
	GameCommands.SendAIN(allianceID)
	parts, err := waiter.WaitWithTimeout()
	if err != nil {
		return roster, err
	}
	if len(parts) <= 5 {
		return roster, fmt.Errorf("AIN response has no payload")
	}
	if err := json.Unmarshal([]byte(parts[5]), &roster); err != nil {
		return roster, err
	}
	if roster.Alliance.AllianceID != allianceID {
		return roster, fmt.Errorf("AIN returned alliance %d", roster.Alliance.AllianceID)
	}
	return roster, nil
}

func matchAINAlliance(allianceID int) func([]string) bool {
	return func(parts []string) bool {
		if len(parts) <= 5 {
			return false
		}
		var envelope struct {
			Alliance struct {
				AllianceID int `json:"AID"`
			} `json:"A"`
		}
		return json.Unmarshal([]byte(parts[5]), &envelope) == nil && envelope.Alliance.AllianceID == allianceID
	}
}

func loadTopAlliances(ctx context.Context, server string) ([]AllianceTargetAlliance, error) {
	allianceListCache.Lock()
	if allianceListCache.Server == server && len(allianceListCache.Rows) == 50 && time.Since(allianceListCache.FetchedAt) < 5*time.Minute {
		rows := append([]AllianceTargetAlliance(nil), allianceListCache.Rows...)
		allianceListCache.Unlock()
		return rows, nil
	}
	allianceListCache.Unlock()

	type pageResult struct {
		page int
		data trackerAlliancePage
		err  error
	}
	results := make(chan pageResult, 4)
	for page := 1; page <= 4; page++ {
		go func(page int) {
			var data trackerAlliancePage
			path := fmt.Sprintf("/alliances?page=%d&orderBy=might_current&orderType=DESC", page)
			err := getAllianceTargetJSON(ctx, server, path, &data)
			results <- pageResult{page: page, data: data, err: err}
		}(page)
	}
	pages := make([]trackerAlliancePage, 4)
	for range 4 {
		result := <-results
		if result.err != nil {
			return nil, result.err
		}
		pages[result.page-1] = result.data
	}
	rows := make([]AllianceTargetAlliance, 0, 50)
	for _, page := range pages {
		for _, row := range page.Alliances {
			rows = append(rows, AllianceTargetAlliance{
				ExternalID:  row.AllianceID,
				AllianceID:  trackerGameID(row.AllianceID),
				Name:        row.Name,
				Rank:        len(rows) + 1,
				Might:       int64(row.Might),
				PlayerCount: int(row.PlayerCount),
			})
			if len(rows) == 50 {
				break
			}
		}
		if len(rows) == 50 {
			break
		}
	}
	allianceListCache.Lock()
	allianceListCache.Server = server
	allianceListCache.FetchedAt = time.Now()
	allianceListCache.Rows = append([]AllianceTargetAlliance(nil), rows...)
	allianceListCache.Unlock()
	return rows, nil
}

func loadAllianceTargets(ctx context.Context, server, externalID string) (trackerAllianceDetail, []trackerCartographyPlayer, error) {
	var detail trackerAllianceDetail
	var cartography []trackerCartographyPlayer
	var detailErr, cartographyErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		detailErr = getAllianceTargetJSON(ctx, server, "/alliances/id/"+url.PathEscape(externalID), &detail)
	}()
	go func() {
		defer wg.Done()
		cartographyErr = getAllianceTargetJSON(ctx, server, "/cartography/id/"+url.PathEscape(externalID), &cartography)
	}()
	wg.Wait()
	if detailErr != nil {
		return detail, nil, detailErr
	}
	if cartographyErr != nil {
		return detail, nil, cartographyErr
	}
	return detail, cartography, nil
}

func getAllianceTargetJSON(ctx context.Context, server, path string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, allianceTargetAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("gge-server", server)
	resp, err := allianceTargetHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("GGE Tracker request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GGE Tracker returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("GGE Tracker response decode failed: %w", err)
	}
	return nil
}

func buildAllianceTargets(detail trackerAllianceDetail, cartography []trackerCartographyPlayer) []AllianceTargetPlayer {
	castlesByName := make(map[string][]AllianceTargetCastle, len(cartography))
	for _, row := range cartography {
		if castles := attackableTargetCastles(row.Castles); len(castles) > 0 {
			castlesByName[strings.ToLower(strings.TrimSpace(row.Name))] = castles
		}
	}
	now := time.Now()
	rows := make([]AllianceTargetPlayer, 0, len(detail.Players))
	for _, player := range detail.Players {
		targets, ok := castlesByName[strings.ToLower(strings.TrimSpace(player.Name))]
		if !ok {
			continue
		}
		birdUntil, _ := time.Parse(time.RFC3339Nano, player.BirdUntil)
		rpt := 0
		if birdUntil.After(now) {
			rpt = int(time.Until(birdUntil).Seconds())
		}
		for _, target := range targets {
			closest, distance, ok := closestOwnedCastle(target.X, target.Y)
			if !ok {
				continue
			}
			rows = append(rows, AllianceTargetPlayer{
				PlayerID:         trackerGameID(player.PlayerID),
				Name:             player.Name,
				Might:            int64(player.Might),
				UnderBird:        rpt > 0,
				RPTSeconds:       rpt,
				BirdUntil:        player.BirdUntil,
				UpdatedAt:        player.UpdatedAt,
				TargetCastle:     target,
				ClosestOwnCastle: closest,
				Distance:         math.Round(distance*10) / 10,
			})
		}
	}
	sortAllianceTargetRows(rows)
	return rows
}

func buildLiveAllianceTargets(live liveAllianceRoster, detail trackerAllianceDetail) []AllianceTargetPlayer {
	detailByPlayerID := make(map[int]struct {
		Might     int64
		UpdatedAt string
	}, len(detail.Players))
	for _, player := range detail.Players {
		detailByPlayerID[trackerGameID(player.PlayerID)] = struct {
			Might     int64
			UpdatedAt string
		}{Might: int64(player.Might), UpdatedAt: player.UpdatedAt}
	}
	rows := make([]AllianceTargetPlayer, 0, len(live.Alliance.Members)*4)
	for _, player := range live.Alliance.Members {
		metadata := detailByPlayerID[player.PlayerID]
		for _, target := range liveAttackableTargetCastles(player.Castles) {
			closest, distance, ok := closestOwnedCastle(target.X, target.Y)
			if !ok {
				continue
			}
			birdUntil := ""
			if player.RPT > 0 {
				birdUntil = time.Now().Add(time.Duration(player.RPT) * time.Second).UTC().Format(time.RFC3339)
			}
			rows = append(rows, AllianceTargetPlayer{
				PlayerID:         player.PlayerID,
				Name:             player.Name,
				Might:            metadata.Might,
				UnderBird:        player.RPT > 0,
				RPTSeconds:       player.RPT,
				BirdUntil:        birdUntil,
				UpdatedAt:        metadata.UpdatedAt,
				TargetCastle:     target,
				ClosestOwnCastle: closest,
				Distance:         math.Round(distance*10) / 10,
			})
		}
	}
	sortAllianceTargetRows(rows)
	return rows
}

func sortAllianceTargetRows(rows []AllianceTargetPlayer) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Distance != rows[j].Distance {
			return rows[i].Distance < rows[j].Distance
		}
		if strings.ToLower(rows[i].Name) != strings.ToLower(rows[j].Name) {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		}
		return rows[i].TargetCastle.Type < rows[j].TargetCastle.Type
	})
}

func attackableTargetCastles(rows [][]int) []AllianceTargetCastle {
	out := make([]AllianceTargetCastle, 0, 4)
	for _, row := range rows {
		if len(row) < 3 || !isAttackableGreenCastleType(row[2]) {
			continue
		}
		out = append(out, AllianceTargetCastle{TypeName: castleSlotTypeName(row[2]), X: row[0], Y: row[1], Type: row[2]})
	}
	return out
}

func liveAttackableTargetCastles(rows [][]int) []AllianceTargetCastle {
	out := make([]AllianceTargetCastle, 0, 4)
	for _, row := range rows {
		if len(row) < 5 || row[0] != 0 || !isAttackableGreenCastleType(row[4]) {
			continue
		}
		out = append(out, AllianceTargetCastle{
			CastleID: row[1],
			TypeName: castleSlotTypeName(row[4]),
			X:        row[2],
			Y:        row[3],
			Type:     row[4],
		})
	}
	return out
}

func enrichTargetCastleNames(rows []AllianceTargetPlayer) {
	namesByCastleID := make(map[int]string)
	namesByCoords := make(map[string]string)
	if nameIndex, err := spyreport.ReadCastleNameIndex(); err == nil {
		for castleID, name := range nameIndex.ByCastleID {
			namesByCastleID[int(castleID)] = name
		}
		for coordinate, name := range nameIndex.ByCoordinate {
			namesByCoords[coordinate] = name
		}
	}
	coordinates := make([][2]int, 0, len(rows))
	for _, row := range rows {
		coordinates = append(coordinates, [2]int{row.TargetCastle.X, row.TargetCastle.Y})
	}
	mapNodes := Models.GetMapState().NodesAt(0, coordinates)
	for key, node := range mapNodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			continue
		}
		namesByCoords[key] = name
		if node.CastleID > 0 {
			namesByCastleID[node.CastleID] = name
		}
	}
	for i := range rows {
		castle := &rows[i].TargetCastle
		if castle.TypeName == "" {
			castle.TypeName = castleSlotTypeName(castle.Type)
		}
		if castle.CastleID > 0 {
			castle.Name = namesByCastleID[castle.CastleID]
		}
		if castle.Name == "" {
			castle.Name = namesByCoords[fmt.Sprintf("%d_%d", castle.X, castle.Y)]
		}
	}
}

func isAttackableGreenCastleType(castleType int) bool {
	switch castleType {
	case 1, 3, 4, 5, 6, 22:
		return true
	default:
		return false
	}
}

func castleSlotTypeName(castleType int) string {
	switch castleType {
	case 1:
		return "Main castle"
	case 3, 6:
		return "Capital"
	case 4:
		return "Outpost"
	case 5, 22:
		return "Metropolis"
	default:
		return "Player castle"
	}
}

func trackerGameID(externalID string) int {
	v, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return int(v / 1000)
}

func ownedGreenCastles() []AllianceTargetCastle {
	gs := Models.GetGameState()
	c := &gs.Castle
	slots := []*castle.PlayerCastleInfo{&c.MainCastle, &c.Outpost1, &c.Outpost2, &c.Outpost3, &c.Metropolis, &c.Capital}
	rows := make([]AllianceTargetCastle, 0, len(slots))
	seen := make(map[int]bool)
	for _, slot := range slots {
		aid := int(slot.Aid)
		if aid <= 0 || seen[aid] {
			continue
		}
		x, y := slot.MapX, slot.MapY
		if x == 0 && y == 0 {
			x, y = slot.Troops.X, slot.Troops.Y
		}
		if x == 0 && y == 0 {
			continue
		}
		seen[aid] = true
		rows = append(rows, AllianceTargetCastle{CastleID: aid, Name: slot.Name, X: x, Y: y})
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.KingdomID != 0 || loc.CastleID <= 0 || seen[loc.CastleID] || (loc.X == 0 && loc.Y == 0) {
			continue
		}
		seen[loc.CastleID] = true
		rows = append(rows, AllianceTargetCastle{CastleID: loc.CastleID, Name: fmt.Sprintf("Castle %d", loc.CastleID), X: loc.X, Y: loc.Y})
	}
	return rows
}

func closestOwnedCastle(x, y int) (AllianceTargetCastle, float64, bool) {
	var best AllianceTargetCastle
	bestDistance := math.MaxFloat64
	for _, own := range ownedGreenCastles() {
		distance := math.Hypot(float64(x-own.X), float64(y-own.Y))
		if distance < bestDistance {
			best, bestDistance = own, distance
		}
	}
	return best, bestDistance, bestDistance != math.MaxFloat64
}

func currentSpyAvailability() SpyAvailability {
	gs := Models.GetGameState()
	main := &gs.Castle.MainCastle
	result := SpyAvailability{
		BuildingRowsLoaded: main.BuildingRowsLoaded,
		SourceCastle: AllianceTargetCastle{
			CastleID: int(main.Aid),
			Name:     main.Name,
			X:        main.MapX,
			Y:        main.MapY,
		},
	}
	if result.SourceCastle.X == 0 && result.SourceCastle.Y == 0 {
		result.SourceCastle.X, result.SourceCastle.Y = main.Troops.X, main.Troops.Y
	}
	for _, building := range main.AllBuildingRows() {
		tavern, ok := tavernCapacityByWID[building.BuildingID]
		if !ok {
			continue
		}
		result.Taverns = append(result.Taverns, tavern)
		result.Capacity += tavern.Capacity
	}
	movements, _, _, _ := gs.Movement.Snapshot()
	for _, movement := range movements {
		if movement.MovementType == 3 && movement.SID == result.SourceCastle.CastleID {
			result.Active += movement.SpyCount
		}
	}
	result.Available = result.Capacity - result.Active
	if result.Available < 0 {
		result.Available = 0
	}
	return result
}

func SendAllianceTargetSpy(targetX, targetY int) (int, error) {
	if !ResponseRegistry.IsGameWebSocketReady() {
		return 0, fmt.Errorf("game connection is not ready")
	}
	initial := currentSpyAvailability()
	if initial.SourceCastle.CastleID <= 0 {
		return 0, fmt.Errorf("main castle is not available")
	}
	if !initial.BuildingRowsLoaded {
		return 0, fmt.Errorf("main castle building data is not loaded")
	}
	if initial.Available <= 0 {
		return 0, fmt.Errorf("no spies are currently available")
	}
	if targetX < 0 || targetY < 0 {
		return 0, fmt.Errorf("invalid target coordinates")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sent := 0
	err := Automation.RunWork(ctx, Automation.WorkItem{
		Request: Automation.Request{
			Owner:        Automation.OwnerManual,
			Priority:     Automation.PriorityManual,
			Reason:       "send alliance target spies",
			Claims:       []Automation.Claim{Automation.CastleClaim(initial.SourceCastle.CastleID, "spy-missions")},
			MaxHold:      10 * time.Second,
			PreemptLower: true,
		},
		Run: func(_ context.Context, lease *Automation.Lease) error {
			spies := currentSpyAvailability()
			if spies.SourceCastle.CastleID != initial.SourceCastle.CastleID || spies.Available <= 0 {
				return fmt.Errorf("no spies are currently available")
			}
			if !GameCommands.QueueFeaturePayload(
				Automation.OwnerManual,
				GameCommands.CSMPayload(spies.SourceCastle.CastleID, targetX, targetY, spies.Available),
				lease,
			) {
				return Automation.ErrWorkCancelled
			}
			sent = spies.Available
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return sent, nil
}
