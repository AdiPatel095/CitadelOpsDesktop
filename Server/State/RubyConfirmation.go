package State

// RubyConfirmationState records gbd.opt/opt.CC2T, not an EUP price quote.
// A new session generation invalidates the previous observation.
type RubyConfirmationState struct {
	Amount     int64
	Known      bool
	Generation uint64
}

func (setting RubyConfirmationState) Current(session SessionState) bool {
	return setting.Known && session.Generation > 0 && setting.Generation == session.Generation && session.LoggedIn && session.SocketReady && (setting.Amount == -1 || setting.Amount > 0)
}
