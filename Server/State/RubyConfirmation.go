package State

// Official OptionsDialogRubyConfirmationItem range; -1 disables confirmation.
const MaximumRubyConfirmationAmount int64 = 1_000_000

func ValidRubyConfirmationAmount(amount int64) bool {
	return amount == -1 || amount >= 1 && amount <= MaximumRubyConfirmationAmount
}

// RubyConfirmationState records gbd.opt/opt.CC2T, not an EUP price quote.
// A new session generation invalidates the previous observation.
type RubyConfirmationState struct {
	Amount     int64
	Known      bool
	Generation uint64
}

func (setting RubyConfirmationState) Current(session SessionState) bool {
	return setting.Known && session.Generation > 0 && setting.Generation == session.Generation && session.LoggedIn && session.SocketReady && ValidRubyConfirmationAmount(setting.Amount)
}
