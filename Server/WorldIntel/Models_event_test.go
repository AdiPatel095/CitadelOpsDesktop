package WorldIntel

import (
	"strings"
	"testing"
	"time"
)

func TestFinalizeBatchNormalizesBackendEventScoreContract(t *testing.T) {
	now := time.Date(2026, time.August, 11, 16, 2, 19, 0, time.UTC)
	batch, err := FinalizeBatch(ObservationBatch{
		WorldID:    "wss://WORLD.EXAMPLE:443/socket",
		CapturedAt: now,
		Alliances: []AllianceObservation{{
			AllianceID: 12, Name: " Example Alliance ", Source: "alliance", ObservedAt: now,
			PublicMetrics: map[string]PublicMetricObservation{
				" EVENT-POINTS ": {
					Label: " Event Points ", Value: 42, Source: "GGE-HIGHSCORE",
					EventID: 88, MetricID: 7, ListType: 47, LeagueID: 2, ObservedAt: now,
				},
			},
		}},
		EventScores: []EventScoreObservation{{
			EventID: 88, EventKey: " NOMAD-INVASION ", EventName: " Nomad Invasion ",
			ListType: 47, LeagueID: -1, BoardKey: " GLOBAL-PLAYERS ",
			PlayerID: 7, PlayerName: " Example Player ", Rank: 3,
			ScoreKnown: false, ScoreUnit: " POINTS ", EventEndsAt: now.Add(24*time.Hour + 91*time.Second),
			Source: "GGE-HIGHSCORE", ObservedAt: now,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.WorldID != "world.example" {
		t.Fatalf("world id = %q", batch.WorldID)
	}
	if len(batch.EventScores) != 1 {
		t.Fatalf("event scores = %#v", batch.EventScores)
	}
	score := batch.EventScores[0]
	if score.EventKey != "nomad-invasion" || score.BoardKey != "global-players" || score.ScoreUnit != "points" || score.Source != "gge-highscore" {
		t.Fatalf("normalized event score = %#v", score)
	}
	if score.ScoreKnown || score.Score != 0 {
		t.Fatalf("rank-only score invented a value: %#v", score)
	}
	if score.RunStartedOn != "2026-08-11" {
		t.Fatalf("run started on = %q", score.RunStartedOn)
	}
	if len(score.OccurrenceID) != 64 || score.OccurrenceID != EventOccurrenceID(batch.WorldID, score.EventID, score.EventEndsAt) {
		t.Fatalf("occurrence id = %q", score.OccurrenceID)
	}
	metric := batch.Alliances[0].PublicMetrics["event-points"]
	if metric.EventID != 88 || metric.MetricID != 7 || metric.Source != "gge-highscore" {
		t.Fatalf("alliance public metric = %#v", metric)
	}
	if len(batch.BatchID) != 64 || strings.Trim(batch.BatchID, "0123456789abcdef") != "" {
		t.Fatalf("batch id = %q", batch.BatchID)
	}
}

func TestFinalizeBatchRejectsInventedRankOnlyScore(t *testing.T) {
	now := time.Date(2026, time.August, 11, 16, 0, 0, 0, time.UTC)
	_, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		EventScores: []EventScoreObservation{{
			EventID: 88, EventKey: "berimond-invasion", EventName: "Berimond Invasion",
			ListType: 47, LeagueID: -1, PlayerID: 7, PlayerName: "Player", Rank: 3,
			Score: 1, ScoreKnown: false, ScoreUnit: "points", EventEndsAt: now.Add(24 * time.Hour),
			Source: "gge-highscore", ObservedAt: now,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "eventScores[0]") {
		t.Fatalf("expected rank-only score validation error, got %v", err)
	}
}

func TestFinalizeBatchPreservesSchemaOnePublicMetricSources(t *testing.T) {
	now := time.Date(2026, time.August, 11, 16, 0, 0, 0, time.UTC)
	for _, source := range []string{"", "legacy-collector"} {
		t.Run(source, func(t *testing.T) {
			batch, err := FinalizeBatch(ObservationBatch{
				WorldID: "world.example", CapturedAt: now,
				Players: []PlayerObservation{{
					PlayerID: 7, Name: "Player", Source: "account", ObservedAt: now,
					PublicMetrics: map[string]PublicMetricObservation{
						"legacy-score": {Label: "Legacy score", Value: 42, Source: source, ObservedAt: now},
					},
				}},
			})
			if err != nil {
				t.Fatalf("schema v1 source %q rejected: %v", source, err)
			}
			if got := batch.Players[0].PublicMetrics["legacy-score"].Source; got != source {
				t.Fatalf("schema v1 source = %q, want %q", got, source)
			}
		})
	}
}
