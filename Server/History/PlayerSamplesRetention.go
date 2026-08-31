package History

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const PlayerSamplesConfigurationSection = "history.playerSamples"

const DefaultPlayerSamplesRecordingIntervalSeconds = int64((time.Hour) / time.Second)

var validPlayerSamplesRecordingIntervalSeconds = map[int64]struct{}{
	60:   {},
	300:  {},
	600:  {},
	900:  {},
	1800: {},
	3600: {},
}

type PlayerSamplesRetention string

const (
	PlayerSamplesRetentionNone      PlayerSamplesRetention = "none"
	PlayerSamplesRetention24Hours   PlayerSamplesRetention = "24h"
	PlayerSamplesRetention7Days     PlayerSamplesRetention = "7d"
	PlayerSamplesRetention30Days    PlayerSamplesRetention = "30d"
	PlayerSamplesRetention90Days    PlayerSamplesRetention = "90d"
	PlayerSamplesRetention100Days   PlayerSamplesRetention = "100d"
	PlayerSamplesRetention1Year     PlayerSamplesRetention = "1y"
	PlayerSamplesRetentionUnlimited PlayerSamplesRetention = "unlimited"
)

// MaxPlayerSamplesRetentionDays is the technical time.Duration boundary, not
// a product cap. Local profiles may choose any positive day count up to this
// value; hosted runtimes still enforce their smaller policy maximum.
const MaxPlayerSamplesRetentionDays = int64((1<<63 - 1) / int64(24*time.Hour))

type PlayerSamplesConfiguration struct {
	Version                  int                    `json:"version"`
	Retention                PlayerSamplesRetention `json:"retention"`
	RecordingIntervalSeconds int64                  `json:"recordingIntervalSeconds,omitempty"`
}

type PlayerSamplesRecordingIntervalOption struct {
	Seconds          int64  `json:"seconds"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	RecordingsPerDay int64  `json:"recordingsPerDay"`
}

type PlayerSamplesRetentionOption struct {
	Value       PlayerSamplesRetention `json:"value"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Days        int64                  `json:"days,omitempty"`
	Recordings  int64                  `json:"recordings,omitempty"`
}

type PlayerSamplesRetentionPolicy struct {
	Revision                 uint64                                 `json:"revision"`
	Configured               PlayerSamplesRetention                 `json:"configured"`
	ConfiguredDays           *int64                                 `json:"configuredDays,omitempty"`
	Effective                PlayerSamplesRetention                 `json:"effective"`
	EffectiveDays            *int64                                 `json:"effectiveDays,omitempty"`
	RecordingIntervalSeconds int64                                  `json:"recordingIntervalSeconds"`
	RecordingIntervalOptions []PlayerSamplesRecordingIntervalOption `json:"recordingIntervalOptions"`
	Hosted                   bool                                   `json:"hosted"`
	Maximum                  PlayerSamplesRetention                 `json:"maximum"`
	MaximumDays              *int64                                 `json:"maximumDays,omitempty"`
	Options                  []PlayerSamplesRetentionOption         `json:"options"`
	Storage                  PlayerSamplesStorageEstimate           `json:"storage"`
}

// PlayerSamplesStoragePolicy is the normalized policy applied atomically by
// capture and compaction. Keeping retention and cadence together prevents a
// stale writer from appending with either an older window or an older bucket.
type PlayerSamplesStoragePolicy struct {
	Retention                PlayerSamplesRetention
	RecordingIntervalSeconds int64
}

var playerSamplesRecordingIntervalOptions = []PlayerSamplesRecordingIntervalOption{
	{Seconds: 60, Label: "1 minute", Description: "Save up to one My Stats recording per minute.", RecordingsPerDay: 24 * 60},
	{Seconds: 300, Label: "5 minutes", Description: "Save up to one My Stats recording every 5 minutes.", RecordingsPerDay: 24 * 12},
	{Seconds: 600, Label: "10 minutes", Description: "Save up to one My Stats recording every 10 minutes.", RecordingsPerDay: 24 * 6},
	{Seconds: 900, Label: "15 minutes", Description: "Save up to one My Stats recording every 15 minutes.", RecordingsPerDay: 24 * 4},
	{Seconds: 1800, Label: "30 minutes", Description: "Save up to one My Stats recording every 30 minutes.", RecordingsPerDay: 24 * 2},
	{Seconds: 3600, Label: "1 hour", Description: "Save up to one My Stats recording per hour.", RecordingsPerDay: 24},
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
		Description: "Keep My Stats recordings for one day at the selected cadence.",
		Days:        1,
		Recordings:  24,
	},
	{
		Value:       PlayerSamplesRetention7Days,
		Label:       "7 days",
		Description: "Keep My Stats recordings for 7 days at the selected cadence.",
		Days:        7,
		Recordings:  7 * 24,
	},
	{
		Value:       PlayerSamplesRetention30Days,
		Label:       "30 days",
		Description: "Keep My Stats recordings for 30 days at the selected cadence.",
		Days:        30,
		Recordings:  30 * 24,
	},
	{
		Value:       PlayerSamplesRetention90Days,
		Label:       "90 days",
		Description: "Keep My Stats recordings for 90 days at the selected cadence.",
		Days:        90,
		Recordings:  90 * 24,
	},
	{
		Value:       PlayerSamplesRetention100Days,
		Label:       "100 days",
		Description: "Keep My Stats recordings for 100 days at the selected cadence.",
		Days:        100,
		Recordings:  100 * 24,
	},
	{
		Value:       PlayerSamplesRetention1Year,
		Label:       "365 days",
		Description: "Keep My Stats recordings for 365 days at the selected cadence.",
		Days:        365,
		Recordings:  365 * 24,
	},
	{
		Value:       PlayerSamplesRetentionUnlimited,
		Label:       "Unlimited",
		Description: "Keep My Stats recordings at the selected cadence without a day limit.",
	},
}

func ResolvePlayerSamplesRetention(raw json.RawMessage, hosted bool) PlayerSamplesRetentionPolicy {
	configured := PlayerSamplesRetention30Days
	recordingIntervalSeconds := DefaultPlayerSamplesRecordingIntervalSeconds
	var value PlayerSamplesConfiguration
	if json.Unmarshal(raw, &value) == nil && value.Version == 1 && validPlayerSamplesRetention(value.Retention) {
		configured = value.Retention
		if ValidPlayerSamplesRecordingIntervalSeconds(value.RecordingIntervalSeconds) {
			recordingIntervalSeconds = value.RecordingIntervalSeconds
		}
	}
	maximum := PlayerSamplesRetentionUnlimited
	if hosted {
		maximum = PlayerSamplesRetention30Days
	}
	effective := configured
	if playerSamplesRetentionRank(effective) > playerSamplesRetentionRank(maximum) {
		effective = maximum
	}
	options := playerSamplesRetentionOptionsForInterval(recordingIntervalSeconds)
	intervalOptions := make([]PlayerSamplesRecordingIntervalOption, len(playerSamplesRecordingIntervalOptions))
	copy(intervalOptions, playerSamplesRecordingIntervalOptions)
	return PlayerSamplesRetentionPolicy{
		Configured:               configured,
		ConfiguredDays:           playerSamplesRetentionDaysPointer(configured),
		Effective:                effective,
		EffectiveDays:            playerSamplesRetentionDaysPointer(effective),
		RecordingIntervalSeconds: recordingIntervalSeconds,
		RecordingIntervalOptions: intervalOptions,
		Hosted:                   hosted,
		Maximum:                  maximum,
		MaximumDays:              playerSamplesRetentionDaysPointer(maximum),
		Options:                  options,
		Storage:                  DefaultPlayerSamplesStorageEstimate(),
	}
}

func playerSamplesRetentionOptionsForInterval(intervalSeconds int64) []PlayerSamplesRetentionOption {
	intervalSeconds = NormalizePlayerSamplesRecordingIntervalSeconds(intervalSeconds)
	options := make([]PlayerSamplesRetentionOption, len(playerSamplesRetentionOptions))
	copy(options, playerSamplesRetentionOptions)
	for index := range options {
		if options[index].Days <= 0 {
			options[index].Recordings = 0
			continue
		}
		options[index].Recordings = options[index].Days * PlayerSamplesRecordingsPerDay(intervalSeconds)
	}
	return options
}

func ValidPlayerSamplesRecordingIntervalSeconds(value int64) bool {
	_, valid := validPlayerSamplesRecordingIntervalSeconds[value]
	return valid
}

func NormalizePlayerSamplesRecordingIntervalSeconds(value int64) int64 {
	if ValidPlayerSamplesRecordingIntervalSeconds(value) {
		return value
	}
	return DefaultPlayerSamplesRecordingIntervalSeconds
}

func PlayerSamplesRecordingIntervalDuration(value int64) time.Duration {
	return time.Duration(NormalizePlayerSamplesRecordingIntervalSeconds(value)) * time.Second
}

func PlayerSamplesRecordingsPerDay(intervalSeconds int64) int64 {
	return int64((24 * time.Hour) / PlayerSamplesRecordingIntervalDuration(intervalSeconds))
}

func NormalizePlayerSamplesStoragePolicy(policy PlayerSamplesStoragePolicy) PlayerSamplesStoragePolicy {
	return PlayerSamplesStoragePolicy{
		Retention:                NormalizePlayerSamplesRetention(policy.Retention),
		RecordingIntervalSeconds: NormalizePlayerSamplesRecordingIntervalSeconds(policy.RecordingIntervalSeconds),
	}
}

func PlayerSamplesStoragePolicyFromRetentionPolicy(policy PlayerSamplesRetentionPolicy) PlayerSamplesStoragePolicy {
	return NormalizePlayerSamplesStoragePolicy(PlayerSamplesStoragePolicy{
		Retention:                policy.Effective,
		RecordingIntervalSeconds: policy.RecordingIntervalSeconds,
	})
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
	value = NormalizePlayerSamplesRetention(value)
	switch value {
	case PlayerSamplesRetentionNone:
		return 0, false
	case PlayerSamplesRetentionUnlimited:
		return 0, true
	}
	days, valid := PlayerSamplesRetentionDays(value)
	if !valid {
		return 30 * 24 * time.Hour, false
	}
	return time.Duration(days) * 24 * time.Hour, false
}

func validPlayerSamplesRetention(value PlayerSamplesRetention) bool {
	switch value {
	case PlayerSamplesRetentionNone, PlayerSamplesRetentionUnlimited:
		return true
	}
	_, valid := PlayerSamplesRetentionDays(value)
	return valid
}

// PlayerSamplesRetentionDays returns the finite whole-day window represented
// by a retention value. Legacy 24h and 1y values remain valid, while new local
// choices use the compatible <days>d form (for example, 100d).
func PlayerSamplesRetentionDays(value PlayerSamplesRetention) (int64, bool) {
	switch value {
	case PlayerSamplesRetention24Hours:
		return 1, true
	case PlayerSamplesRetention1Year:
		return 365, true
	}
	raw := strings.TrimSpace(string(value))
	if len(raw) < 2 || !strings.HasSuffix(raw, "d") {
		return 0, false
	}
	days, err := strconv.ParseInt(strings.TrimSuffix(raw, "d"), 10, 64)
	if err != nil || days < 1 || days > MaxPlayerSamplesRetentionDays {
		return 0, false
	}
	return days, true
}

func PlayerSamplesRetentionForDays(days int64) (PlayerSamplesRetention, bool) {
	if days < 1 || days > MaxPlayerSamplesRetentionDays {
		return "", false
	}
	return PlayerSamplesRetention(strconv.FormatInt(days, 10) + "d"), true
}

func playerSamplesRetentionDaysPointer(value PlayerSamplesRetention) *int64 {
	days, valid := PlayerSamplesRetentionDays(value)
	if !valid {
		return nil
	}
	return &days
}

func playerSamplesRetentionRank(value PlayerSamplesRetention) int64 {
	value = NormalizePlayerSamplesRetention(value)
	switch value {
	case PlayerSamplesRetentionNone:
		return 0
	case PlayerSamplesRetentionUnlimited:
		return MaxPlayerSamplesRetentionDays + 1
	}
	days, valid := PlayerSamplesRetentionDays(value)
	if !valid {
		return 30
	}
	return days
}
