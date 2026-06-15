package featureview

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
)

type riftCRAJob struct {
	cancel       context.CancelFunc
	arriveAtUnix int64
	commanderID  int
	sourceX      int
	sourceY      int
}

var (
	riftCRAScheduleMu   sync.Mutex
	riftCRAScheduleJobs = make(map[string]*riftCRAJob)
)

func init() {
	GameParser.ScheduledRiftCRAArrivals = scheduledRiftCRAArrivalMap
}

func scheduledRiftCRAArrivalMap() map[string]int64 {
	riftCRAScheduleMu.Lock()
	defer riftCRAScheduleMu.Unlock()
	out := make(map[string]int64, len(riftCRAScheduleJobs))
	for id, job := range riftCRAScheduleJobs {
		if job != nil {
			out[id] = job.arriveAtUnix
		}
	}
	return out
}

// CancelRiftCRASchedule cancels a pending scheduled resend for one launch id.
func CancelRiftCRASchedule(launchID string) {
	cancelRiftCRASchedule(launchID, true)
}

// CancelRiftCRAScheduleQuiet cancels without pushing a websocket refresh (caller will broadcast after other updates).
func CancelRiftCRAScheduleQuiet(launchID string) {
	cancelRiftCRASchedule(launchID, false)
}

func cancelRiftCRASchedule(launchID string, notify bool) {
	riftCRAScheduleMu.Lock()
	if job, ok := riftCRAScheduleJobs[launchID]; ok && job != nil && job.cancel != nil {
		job.cancel()
		Logging.RiftLogf("schedule_cancel", "launch=%s", launchID)
	}
	delete(riftCRAScheduleJobs, launchID)
	riftCRAScheduleMu.Unlock()

	// Notify builds the wire payload (reads scheduled jobs) — must not run while holding riftCRAScheduleMu.
	if notify && GameParser.NotifyRiftCRALaunchChanged != nil {
		GameParser.NotifyRiftCRALaunchChanged()
	}
}

// ScheduleRiftCRALaunch queues a resend so the attack arrives at arriveAtUnix (local wall clock).
func ScheduleRiftCRALaunch(launchID string, arriveAtUnix int64, commanderID, sourceX, sourceY int) error {
	launch, ok := riftattack.FindLaunch(launchID)
	if !ok {
		return fmt.Errorf("no saved Rift CRA launch template")
	}
	tt := launch.OneWayTTSeconds
	if tt <= 0 {
		return fmt.Errorf("no feather travel time yet — complete one successful feather launch first")
	}

	arriveAtUnix = GameParser.NormalizeScheduledArriveAt(arriveAtUnix, tt)
	minArrive := GameParser.RoundUpToUnixMinute(time.Now().Unix() + int64(tt))
	if arriveAtUnix < minArrive {
		return fmt.Errorf("arrival time is earlier than feather travel allows (min %s)", time.Unix(minArrive, 0).Format("15:04:05"))
	}

	nowUnix := time.Now().Unix()
	fireAt := arriveAtUnix - int64(tt)
	if fireAt < nowUnix {
		fireAt = nowUnix
	}

	cancelRiftCRASchedule(launchID, false)

	ctx, cancel := context.WithCancel(context.Background())
	job := &riftCRAJob{
		cancel:       cancel,
		arriveAtUnix: arriveAtUnix,
		commanderID:  commanderID,
		sourceX:      sourceX,
		sourceY:      sourceY,
	}
	riftCRAScheduleMu.Lock()
	riftCRAScheduleJobs[launchID] = job
	riftCRAScheduleMu.Unlock()

	delay := time.Until(time.Unix(fireAt, 0))
	Logging.RiftLogf("schedule", "launch=%s fire in %s arrive %s TT=%ds",
		launchID, delay.Round(time.Second), time.Unix(arriveAtUnix, 0).Format("15:04:05"), tt)

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			Logging.RiftLogf("schedule_cancel", "launch=%s (timer aborted)", launchID)
			return
		case <-timer.C:
		}

		riftCRAScheduleMu.Lock()
		delete(riftCRAScheduleJobs, launchID)
		riftCRAScheduleMu.Unlock()

		gs := Models.GetGameState()
		effectiveCommander := commanderID
		if effectiveCommander < 0 {
			effectiveCommander = riftattack.CommanderIDFromLaunch(launch)
		}
		if effectiveCommander >= 0 && gs.IsCommanderWireIDBusy(effectiveCommander) {
			Logging.RiftLogf("schedule_skip", "launch=%s commander LID=%d busy", launchID, effectiveCommander)
			if GameParser.NotifyRiftCRALaunchChanged != nil {
				GameParser.NotifyRiftCRALaunchChanged()
			}
			return
		}

		if err := GameParser.ReplaySavedRiftCRA(launchID, commanderID, sourceX, sourceY); err != nil {
			Logging.RiftLogf("schedule_failed", "launch=%s: %v", launchID, err)
			return
		}
		Logging.RiftLogf("schedule_fire", "launch=%s", launchID)
	}()

	if GameParser.NotifyRiftCRALaunchChanged != nil {
		GameParser.NotifyRiftCRALaunchChanged()
	}
	return nil
}

// ReplayOrScheduleRiftCRA sends immediately when arrival is ASAP, otherwise schedules for arriveAtUnix.
func ReplayOrScheduleRiftCRA(launchID string, arriveAtUnix int64, commanderID, sourceX, sourceY int) error {
	launch, ok := riftattack.FindLaunch(launchID)
	if !ok {
		Logging.RiftLogf("resend_failed", "launch %q not found", launchID)
		return fmt.Errorf("no saved Rift CRA launch template")
	}
	tt := launch.OneWayTTSeconds
	if tt <= 0 || arriveAtUnix <= 0 {
		Logging.RiftLogf("resend_request", "launch=%s immediate (tt=%d arriveAt=%d)", launchID, tt, arriveAtUnix)
		return GameParser.ReplaySavedRiftCRA(launchID, commanderID, sourceX, sourceY)
	}
	minArrive := GameParser.RoundUpToUnixMinute(time.Now().Unix() + int64(tt))
	normalized := GameParser.NormalizeScheduledArriveAt(arriveAtUnix, tt)
	if normalized <= minArrive {
		Logging.RiftLogf("resend_request", "launch=%s immediate arriveAt=%d minArrive=%d", launchID, arriveAtUnix, minArrive)
		CancelRiftCRASchedule(launchID)
		return GameParser.ReplaySavedRiftCRA(launchID, commanderID, sourceX, sourceY)
	}
	Logging.RiftLogf("resend_request", "launch=%s schedule arriveAt=%d (normalized %d)", launchID, arriveAtUnix, normalized)
	return ScheduleRiftCRALaunch(launchID, normalized, commanderID, sourceX, sourceY)
}
