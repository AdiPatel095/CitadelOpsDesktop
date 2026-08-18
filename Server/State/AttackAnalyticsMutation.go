package State

type attackAnalyticsMutationPart uint8

const (
	attackAnalyticsLaunchIDs attackAnalyticsMutationPart = 1 << iota
	attackAnalyticsPending
	attackAnalyticsRecentStorm
)

func (state *GameState) prepareAttackAnalyticsMutation(source GameState) {
	state.AttackAnalytics = source.AttackAnalytics
	state.attackAnalyticsMutationCOW = true
	state.mutableAttackAnalyticsParts = 0
}

func (state *GameState) MutableAttackAnalyticsLaunchIDs() []MovementID {
	if state == nil {
		return nil
	}
	if state.attackAnalyticsMutationCOW && state.mutableAttackAnalyticsParts&attackAnalyticsLaunchIDs == 0 {
		state.AttackAnalytics.LaunchIDs = append([]MovementID(nil), state.AttackAnalytics.LaunchIDs...)
		state.mutableAttackAnalyticsParts |= attackAnalyticsLaunchIDs
	}
	return state.AttackAnalytics.LaunchIDs
}

func (state *GameState) SetAttackAnalyticsLaunchIDs(values []MovementID) {
	if state == nil {
		return
	}
	state.AttackAnalytics.LaunchIDs = values
	if state.attackAnalyticsMutationCOW {
		state.mutableAttackAnalyticsParts |= attackAnalyticsLaunchIDs
	}
}

func (state *GameState) MutablePendingAttackAnalytics() []AttackFeatureLaunch {
	if state == nil {
		return nil
	}
	if state.attackAnalyticsMutationCOW && state.mutableAttackAnalyticsParts&attackAnalyticsPending == 0 {
		state.AttackAnalytics.PendingAttacks = append([]AttackFeatureLaunch(nil), state.AttackAnalytics.PendingAttacks...)
		state.mutableAttackAnalyticsParts |= attackAnalyticsPending
	}
	return state.AttackAnalytics.PendingAttacks
}

func (state *GameState) SetPendingAttackAnalytics(values []AttackFeatureLaunch) {
	if state == nil {
		return
	}
	state.AttackAnalytics.PendingAttacks = values
	if state.attackAnalyticsMutationCOW {
		state.mutableAttackAnalyticsParts |= attackAnalyticsPending
	}
}

func (state *GameState) MutableRecentAutoStormLaunches() []AttackFeatureLaunch {
	if state == nil {
		return nil
	}
	if state.attackAnalyticsMutationCOW && state.mutableAttackAnalyticsParts&attackAnalyticsRecentStorm == 0 {
		state.AttackAnalytics.RecentAutoStormLaunches = append(
			[]AttackFeatureLaunch(nil), state.AttackAnalytics.RecentAutoStormLaunches...,
		)
		state.mutableAttackAnalyticsParts |= attackAnalyticsRecentStorm
	}
	return state.AttackAnalytics.RecentAutoStormLaunches
}

func (state *GameState) SetRecentAutoStormLaunches(values []AttackFeatureLaunch) {
	if state == nil {
		return
	}
	state.AttackAnalytics.RecentAutoStormLaunches = values
	if state.attackAnalyticsMutationCOW {
		state.mutableAttackAnalyticsParts |= attackAnalyticsRecentStorm
	}
}
