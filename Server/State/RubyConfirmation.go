package State

import (
	"encoding/json"
	"time"
)

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
	ObservedAt time.Time
}

func (setting RubyConfirmationState) Current(session SessionState) bool {
	return setting.Known && session.Generation > 0 && setting.Generation == session.Generation && session.LoggedIn && session.SocketReady && ValidRubyConfirmationAmount(setting.Amount)
}

// RubyConfirmationBlocks uses the same session-scoped rule for every ruby guard.
func RubyConfirmationBlocks(setting RubyConfirmationState, session SessionState, cost int64) (bool, string) {
	if !setting.Current(session) {
		return true, "ruby_confirmation_unknown"
	}
	if setting.Amount > 0 && cost >= setting.Amount {
		return true, "ruby_confirmation_required"
	}
	return false, ""
}

// RubyConfirmationPurchaseHeld releases a rejected occurrence only after a new,
// permissive setting observation. A purchase quote never supplies that setting.
func RubyConfirmationPurchaseHeld(setting RubyConfirmationState, session SessionState, record GlobalEffectPurchaseRecord, cost int64) bool {
	if record.Outcome != GlobalEffectPurchaseConfirmationRequired {
		return false
	}
	blocked, _ := RubyConfirmationBlocks(setting, session, cost)
	return blocked || !setting.ObservedAt.After(record.ResultObservedAt)
}

// RubyConfirmationQuote validates only a positive integer purchase quote.
func RubyConfirmationQuote(payload json.RawMessage) (int64, bool) {
	var quote struct {
		CC2T *int64 `json:"CC2T"`
	}
	if json.Unmarshal(payload, &quote) != nil || quote.CC2T == nil || *quote.CC2T <= 0 {
		return 0, false
	}
	return *quote.CC2T, true
}
