package Buildings

import (
	"CitadelDesktop/Server/Localization"
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
		return &Blocker{Code: "ruby_confirmation_unknown", Message: "Upgrade skipped because its official ruby cost is unavailable or malformed.", MessageDescriptor: Localization.Bind(Localization.New("server.buildings.ruby_cost_unknown", "Upgrade skipped because its official ruby cost is unavailable or malformed.", nil), "Upgrade skipped because its official ruby cost is unavailable or malformed.")}
	}
	setting := state.Player.RubyConfirmation
	if !setting.Current(state.Session) {
		message := fmt.Sprintf("Upgrade skipped: the current game ruby confirmation amount is unavailable. Upgrade cost: %s. Refresh the game settings before retrying.", RubyAmount(int64(amount)))
		return &Blocker{Code: "ruby_confirmation_unknown", Message: message, MessageDescriptor: Localization.Bind(Localization.New("server.buildings.ruby_setting_unknown", "Upgrade skipped: the current game ruby confirmation amount is unavailable. Upgrade cost (rubies): {cost, number}. Refresh the game settings before retrying.", Localization.Params{"cost": int64(amount)}), message)}
	}
	if setting.Amount > 0 && int64(amount) >= setting.Amount {
		message := fmt.Sprintf("Your ruby confirmation amount is too low for this upgrade. Upgrade cost: %s; game confirmation amount: %s.", RubyAmount(int64(amount)), RubyAmount(setting.Amount))
		return &Blocker{Code: "ruby_confirmation_required", Message: message, MessageDescriptor: Localization.Bind(Localization.New("server.buildings.ruby_confirmation_required", "Your ruby confirmation amount is too low for this upgrade. Upgrade cost (rubies): {cost, number}; game confirmation amount (rubies): {threshold, number}.", Localization.Params{"cost": int64(amount), "threshold": setting.Amount}), message)}
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
