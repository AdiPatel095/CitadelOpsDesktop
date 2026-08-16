package State

// These domains are keyed by small official identities and reducers replace
// complete values. Selective mutations therefore need to own only the index;
// immutable nested values stay shared with the preceding generation. The
// broad compatibility mutation path still performs a defensive deep clone.

func (state *GameState) preparePlayerMutation(source GameState) {
	state.Player = source.Player
	state.Player.Resources = cloneMap(source.Player.Resources)
	state.Player.Currencies = cloneMap(source.Player.Currencies)
}

func (state *GameState) prepareCommanderMutation(source GameState) {
	state.Commanders = cloneMap(source.Commanders)
}

func (state *GameState) prepareCastellanMutation(source GameState) {
	state.Castellans = cloneMap(source.Castellans)
}

func (state *GameState) prepareGeneralMutation(source GameState) {
	state.Generals = cloneMap(source.Generals)
}

func (state *GameState) prepareStationingMutation(source GameState) {
	state.Stationing = cloneMap(source.Stationing)
}

func (state *GameState) prepareMarketMutation(source GameState) {
	state.Market = source.Market
	state.Market.Castles = cloneMap(source.Market.Castles)
	state.Market.Boosters = cloneMap(source.Market.Boosters)
}

func (state *GameState) prepareAttackDialogMutation(source GameState) {
	state.AttackDialog = source.AttackDialog
	state.AttackDialog.ActiveEffects = append([]AttackDialogEffect(nil), source.AttackDialog.ActiveEffects...)
}

func (state *GameState) prepareAttackPresetMutation(source GameState) {
	state.AttackPresets = append([]AttackPreset(nil), source.AttackPresets...)
}

func (state *GameState) prepareAutomationMutation(source GameState) {
	state.Automations = cloneMap(source.Automations)
}

func (state *GameState) prepareObservationMutation(source GameState) {
	state.Observations = cloneMap(source.Observations)
}
