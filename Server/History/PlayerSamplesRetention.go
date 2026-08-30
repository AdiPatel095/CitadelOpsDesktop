package History

import (
	"encoding/json"
	"time"
)

const PlayerSamplesConfigurationSection = "history.playerSamples"

type PlayerSamplesRetention string

const (
	PlayerSamplesRetentionNone      PlayerSamplesRetention = "none"
	PlayerSamplesRetention24Hours   PlayerSamplesRetention = "24h"
	PlayerSamplesRetention7Days     PlayerSamplesRetention = "7d"
	PlayerSamplesRetention30Days    PlayerSamplesRetention = "30d"
	PlayerSamplesRetention90Days    PlayerSamplesRetention = "90d"
	PlayerSamplesRetention1Year     PlayerSamplesRetention = "1y"
	PlayerSamplesRetentionUnlimited PlayerSamplesRetention = "unlimited"
)

type PlayerSamplesConfiguration struct {
	Version   int                    `json:"version"`
	Retention PlayerSamplesRetention `json:"retention"`
}

type PlayerSamplesRetentionOption struct {
	Value       PlayerSamplesRetention `json:"value"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
}

type PlayerSamplesRetentionPolicy struct {
	Revision   uint64                         `json:"revision"`
	Configured PlayerSamplesRetention         `json:"configured"`
	Effective  PlayerSamplesRetention         `json:"effective"`
	Hosted     bool                           `json:"hosted"`
	Maximum    PlayerSamplesRetention         `json:"maximum"`
	Options    []PlayerSamplesRetentionOption `json:"options"`
}

var playerSamplesRetentionOptions = []PlayerSamplesRetentionOption{
	{
		Value:       PlayerSamplesRetentionNone,
		Label:       "No history",
		Description: "Do not save historical My Stats points. Current values remain available.",
	},
	{
		Value:       PlayerSamplesRetention24Hours,
		Label:       "24 hours",
		Description: "Keep one-minute My Stats points for the past 24 hours.",
	},
	{
		Value:       PlayerSamplesRetention7Days,
		Label:       "7 days",
		Description: "Keep one-minute points for 24 hours, then one point per hour through day 7.",
	},
	{
		Value:       PlayerSamplesRetention30Days,
		Label:       "30 days",
		Description: "Keep hourly history through day 7, then one point per day through day 30.",
	},
	{
		Value:       PlayerSamplesRetention90Days,
		Label:       "90 days",
		Description: "Keep hourly history through day 7, then one point per day through day 90.",
	},
	{
		Value:       PlayerSamplesRetention1Year,
		Label:       "1 year",
		Description: "Keep hourly history through day 7, then one point per day for one year.",
	},
	{
		Value:       PlayerSamplesRetentionUnlimited,
		Label:       "Unlimited",
		Description: "Keep hourly history through day 7, then one point per day without a time limit.",
	},
}

func ResolvePlayerSamplesRetention(raw json.RawMessage, hosted bool) PlayerSamplesRetentionPolicy {
	configured := PlayerSamplesRetention30Days
	var value PlayerSamplesConfiguration
	if json.Unmarshal(raw, &value) == nil && value.Version == 1 && validPlayerSamplesRetention(value.Retention) {
		configured = value.Retention
	}
	maximum := PlayerSamplesRetentionUnlimited
	if hosted {
		maximum = PlayerSamplesRetention30Days
	}
	effective := configured
	if playerSamplesRetentionRank(effective) > playerSamplesRetentionRank(maximum) {
		effective = maximum
	}
	options := make([]PlayerSamplesRetentionOption, len(playerSamplesRetentionOptions))
	copy(options, playerSamplesRetentionOptions)
	return PlayerSamplesRetentionPolicy{
		Configured: configured,
		Effective:  effective,
		Hosted:     hosted,
		Maximum:    maximum,
		Options:    options,
	}
}

func NormalizePlayerSamplesRetention(value PlayerSamplesRetention) PlayerSamplesRetention {
	if validPlayerSamplesRetention(value) {
		return value
	}
	return PlayerSamplesRetention30Days
}

func ValidPlayerSamplesRetention(value PlayerSamplesRetention) bool {
	return validPlayerSamplesRetention(value)
}

func PlayerSamplesRetentionDuration(value PlayerSamplesRetention) (time.Duration, bool) {
	switch NormalizePlayerSamplesRetention(value) {
	case PlayerSamplesRetentionNone:
		return 0, false
	case PlayerSamplesRetention24Hours:
		return 24 * time.Hour, false
	case PlayerSamplesRetention7Days:
		return 7 * 24 * time.Hour, false
	case PlayerSamplesRetention30Days:
		return 30 * 24 * time.Hour, false
	case PlayerSamplesRetention90Days:
		return 90 * 24 * time.Hour, false
	case PlayerSamplesRetention1Year:
		return 365 * 24 * time.Hour, false
	case PlayerSamplesRetentionUnlimited:
		return 0, true
	default:
		return 30 * 24 * time.Hour, false
	}
}

func validPlayerSamplesRetention(value PlayerSamplesRetention) bool {
	switch value {
	case PlayerSamplesRetentionNone,
		PlayerSamplesRetention24Hours,
		PlayerSamplesRetention7Days,
		PlayerSamplesRetention30Days,
		PlayerSamplesRetention90Days,
		PlayerSamplesRetention1Year,
		PlayerSamplesRetentionUnlimited:
		return true
	default:
		return false
	}
}

func playerSamplesRetentionRank(value PlayerSamplesRetention) int {
	switch NormalizePlayerSamplesRetention(value) {
	case PlayerSamplesRetentionNone:
		return 0
	case PlayerSamplesRetention24Hours:
		return 1
	case PlayerSamplesRetention7Days:
		return 2
	case PlayerSamplesRetention30Days:
		return 3
	case PlayerSamplesRetention90Days:
		return 4
	case PlayerSamplesRetention1Year:
		return 5
	case PlayerSamplesRetentionUnlimited:
		return 6
	default:
		return 3
	}
}
