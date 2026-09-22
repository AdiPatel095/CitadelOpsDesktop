package Buildings

import (
	"CitadelDesktop/Server/State"
	"fmt"
	"math"
	"strconv"
)

func RubyUpgradeBlocker(state State.GameState, costs []CostStatus) *Blocker {
	amount := float64(0)
	for _, cost := range costs {
		if cost.Premium {
			amount += cost.Required
		}
	}
	if amount == 0 {
		return nil
	}
	if amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || amount != math.Trunc(amount) || amount >= math.Exp2(63) {
		return &Blocker{Code: "ruby_confirmation_unknown", Message: "Upgrade skipped because its official ruby cost is unavailable or malformed."}
	}
	setting := state.Player.RubyConfirmation
	if !setting.Current(state.Session) {
		return &Blocker{Code: "ruby_confirmation_unknown", Message: fmt.Sprintf("Upgrade skipped: the current game ruby confirmation amount is unavailable. Upgrade cost: %s. Refresh the game settings before retrying.", RubyAmount(int64(amount)))}
	}
	if setting.Amount > 0 && int64(amount) >= setting.Amount {
		return &Blocker{Code: "ruby_confirmation_required", Message: fmt.Sprintf("Your ruby confirmation amount is too low for this upgrade. Upgrade cost: %s; game confirmation amount: %s.", RubyAmount(int64(amount)), RubyAmount(setting.Amount))}
	}
	return nil
}

func RubyAmount(amount int64) string {
	text := strconv.FormatInt(amount, 10)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	if amount == 1 {
		return text + " ruby"
	}
	return text + " rubies"
}

func IsRubyUpgradeBlocker(code string) bool {
	return code == "premium_disallowed" || code == "ruby_confirmation_unknown" || code == "ruby_confirmation_required"
}
