package Automation

import "sort"

type CommandLaneStatus struct {
	Lane     Lane      `json:"lane"`
	Queued   []Command `json:"queued"`
	InFlight []Command `json:"inFlight"`
}

type ControlStatus struct {
	Coordinator CoordinatorStatus     `json:"coordinator"`
	Work        []WorkStatus          `json:"work"`
	Commands    []CommandLaneStatus   `json:"commands"`
	State       map[string]StateStamp `json:"state"`
}

func ControlSnapshot() ControlStatus {
	queued := Commands.Snapshot()
	inFlight := Commands.InFlightSnapshot()
	lanes := []Lane{LaneCommand, LaneAttackLaunch}
	commandStatus := make([]CommandLaneStatus, 0, len(lanes))
	for _, lane := range lanes {
		laneStatus := CommandLaneStatus{
			Lane:   lane,
			Queued: queued[lane],
		}
		for _, command := range inFlight {
			if command.Lane == lane {
				laneStatus.InFlight = append(laneStatus.InFlight, command)
			}
		}
		sort.Slice(laneStatus.Queued, func(i, j int) bool { return commandBefore(laneStatus.Queued[i], laneStatus.Queued[j]) })
		sort.Slice(laneStatus.InFlight, func(i, j int) bool { return laneStatus.InFlight[i].ID < laneStatus.InFlight[j].ID })
		commandStatus = append(commandStatus, laneStatus)
	}
	coordinator := Snapshot()
	sort.Slice(coordinator.Active, func(i, j int) bool { return coordinator.Active[i].ID < coordinator.Active[j].ID })
	sort.Slice(coordinator.Queued, func(i, j int) bool {
		if coordinator.Queued[i].Priority != coordinator.Queued[j].Priority {
			return coordinator.Queued[i].Priority > coordinator.Queued[j].Priority
		}
		return coordinator.Queued[i].QueuedAt.Before(coordinator.Queued[j].QueuedAt)
	})
	return ControlStatus{
		Coordinator: coordinator,
		Work:        Work.Snapshot(),
		Commands:    commandStatus,
		State:       State.All(),
	}
}
