package WorldIntel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/State"
)

const captureBucket = 15 * time.Minute

func BuildObservationBatch(snapshot State.GameState, capturedAt time.Time) (ObservationBatch, bool, error) {
	if !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady ||
		snapshot.Session.BaselineGeneration != snapshot.Session.Generation {
		return ObservationBatch{}, false, nil
	}
	worldID := NormalizeWorldID(snapshot.Account.WorldID)
	if worldID == "" {
		worldID = NormalizeWorldID(snapshot.Session.ServerURL)
	}
	if worldID == "" {
		return ObservationBatch{}, false, nil
	}
	capturedAt = capturedAt.UTC().Truncate(captureBucket)
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC().Truncate(captureBucket)
	}

	players := map[int64]PlayerObservation{}
	alliances := map[int64]AllianceObservation{}
	holdings := map[string]HoldingObservation{}
	rankings := map[string]EventRankingObservation{}

	allianceDirectory := make(map[State.AllianceID]State.AllianceState, len(snapshot.Alliances)+1)
	for id, alliance := range snapshot.Alliances {
		if alliance.ID <= 0 {
			alliance.ID = id
		}
		if alliance.ID > 0 {
			allianceDirectory[alliance.ID] = alliance
		}
	}
	if snapshot.Alliance.ID > 0 {
		allianceDirectory[snapshot.Alliance.ID] = snapshot.Alliance
	}

	for _, alliance := range allianceDirectory {
		if alliance.ID <= 0 || strings.TrimSpace(alliance.Name) == "" {
			continue
		}
		observedAt := observationTime(alliance.ObservedAt, capturedAt)
		totalMight := 0.0
		for _, member := range alliance.Members {
			totalMight += max(member.Might, 0)
			mergePlayerObservation(players, PlayerObservation{
				WorldID: worldID, PlayerID: int64(member.PlayerID), Name: member.Name,
				AllianceID: int64(alliance.ID), AllianceName: alliance.Name,
				Level: member.Level, LegendLevel: member.LegendLevel, Might: max(member.Might, 0),
				Source: "alliance", ObservedAt: observedAt,
			})
		}
		alliances[int64(alliance.ID)] = AllianceObservation{
			WorldID: worldID, AllianceID: int64(alliance.ID), Name: alliance.Name,
			MemberCount: len(alliance.Members), TotalMight: totalMight,
			Source: "alliance", ObservedAt: observedAt,
		}
		for _, holding := range alliance.Holdings {
			if holding.CastleID <= 0 || holding.PlayerID <= 0 || holding.X < 0 || holding.Y < 0 {
				continue
			}
			key := fmt.Sprintf("%d:%d", alliance.ID, holding.CastleID)
			holdings[key] = HoldingObservation{
				WorldID: worldID, AllianceID: int64(alliance.ID), PlayerID: int64(holding.PlayerID),
				CastleID: int64(holding.CastleID), KingdomID: int64(holding.KingdomID),
				X: holding.X, Y: holding.Y, SlotType: holding.SlotType, ObservedAt: observedAt,
			}
		}
	}

	if snapshot.Player.ID > 0 && strings.TrimSpace(snapshot.Player.Name) != "" {
		allianceName := ""
		if alliance, found := allianceDirectory[snapshot.Player.AllianceID]; found {
			allianceName = alliance.Name
		}
		mergePlayerObservation(players, PlayerObservation{
			WorldID: worldID, PlayerID: int64(snapshot.Player.ID), Name: snapshot.Player.Name,
			AllianceID: int64(snapshot.Player.AllianceID), AllianceName: allianceName,
			Level: snapshot.Player.Level, LegendLevel: snapshot.Player.LegendLevel,
			Might: max(snapshot.Player.Might, 0), Glory: max(snapshot.Player.Glory, 0),
			Source: "account", ObservedAt: capturedAt,
		})
	}

	for eventID, ranking := range snapshot.EventScores.RankingByEvent {
		if ranking.Pending || eventID <= 0 || ranking.ObservedAt.IsZero() {
			continue
		}
		observedAt := observationTime(ranking.ObservedAt, capturedAt)
		for _, entry := range ranking.Entries {
			if entry.AllianceID <= 0 || entry.Rank <= 0 || strings.TrimSpace(entry.Alliance) == "" {
				continue
			}
			key := fmt.Sprintf("%d:%d:%d", eventID, entry.AllianceID, entry.Rank)
			rankings[key] = EventRankingObservation{
				WorldID: worldID, EventID: eventID, AllianceID: int64(entry.AllianceID),
				AllianceName: entry.Alliance, Rank: entry.Rank, Score: max(entry.Score, 0),
				MemberCount: max(entry.MemberCount, 0), FamePoints: max(entry.FamePoints, 0),
				ObservedAt: observedAt,
			}
			if _, found := alliances[int64(entry.AllianceID)]; !found {
				alliances[int64(entry.AllianceID)] = AllianceObservation{
					WorldID: worldID, AllianceID: int64(entry.AllianceID), Name: entry.Alliance,
					MemberCount: int(max(entry.MemberCount, 0)), Source: "event-ranking", ObservedAt: observedAt,
				}
			}
		}
	}

	batch := ObservationBatch{
		SchemaVersion: SchemaVersion, WorldID: worldID, CapturedAt: capturedAt,
		Players: mapValues(players), Alliances: mapValues(alliances),
		Holdings: mapValues(holdings), EventRankings: mapValues(rankings),
	}
	if len(batch.Players)+len(batch.Alliances)+len(batch.Holdings)+len(batch.EventRankings) == 0 {
		return ObservationBatch{}, false, nil
	}
	if len(batch.Players) > MaximumPlayers {
		batch.Players = batch.Players[:MaximumPlayers]
	}
	if len(batch.Alliances) > MaximumAlliances {
		batch.Alliances = batch.Alliances[:MaximumAlliances]
	}
	if len(batch.Holdings) > MaximumHoldings {
		batch.Holdings = batch.Holdings[:MaximumHoldings]
	}
	if len(batch.EventRankings) > MaximumEventRankings {
		batch.EventRankings = batch.EventRankings[:MaximumEventRankings]
	}
	finalized, err := FinalizeBatch(batch)
	if err != nil {
		return ObservationBatch{}, false, err
	}
	return finalized, true, nil
}

func mergePlayerObservation(target map[int64]PlayerObservation, incoming PlayerObservation) {
	if incoming.PlayerID <= 0 || strings.TrimSpace(incoming.Name) == "" {
		return
	}
	current, found := target[incoming.PlayerID]
	if !found {
		target[incoming.PlayerID] = incoming
		return
	}
	if incoming.Source == "account" || incoming.ObservedAt.After(current.ObservedAt) {
		if incoming.AllianceID == 0 {
			incoming.AllianceID = current.AllianceID
			incoming.AllianceName = current.AllianceName
		}
		if incoming.Glory == 0 {
			incoming.Glory = current.Glory
		}
		target[incoming.PlayerID] = incoming
		return
	}
	if current.Glory == 0 && incoming.Glory > 0 {
		current.Glory = incoming.Glory
	}
	target[incoming.PlayerID] = current
}

func observationTime(value time.Time, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}

func mapValues[K comparable, V any](source map[K]V) []V {
	values := make([]V, 0, len(source))
	for _, value := range source {
		values = append(values, value)
	}
	// FinalizeBatch performs domain-specific sorting. Sorting the JSON encoding
	// before limits are applied keeps collector truncation deterministic too.
	sort.SliceStable(values, func(i, j int) bool {
		left, _ := fmt.Sprint(values[i]), i
		right, _ := fmt.Sprint(values[j]), j
		return left < right
	})
	return values
}
