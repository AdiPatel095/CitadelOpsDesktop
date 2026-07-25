package Intent

import (
	"testing"

	"CitadelDesktop/Server/State"
)

// The Auto Khan lanes are one continuous loop: the attack chain never stops, a
// cooldown skip follows every landed report, the wall is restocked after every
// retaliation, and the taunt has to go out the moment the rage bar fills. Those
// lanes must never wait on each other's claims while the protection intents
// still exclude every one of them at once.
func TestKhanLaneClaimsIsolateLanesButYieldToProtection(t *testing.T) {
	gameState := State.NewGameState()
	const main = "100"
	const separateAttackCastle = "200"
	lanes := map[string][]string{
		"attackFromMain": {
			"attack-context", "attack-inventory:" + main, "khan-lane:attack", "commander:7",
		},
		"attackFromOther": {
			"castle-focus", "attack-context", "attack-inventory:" + separateAttackCastle,
			"khan-lane:attack", "commander:7",
		},
		"cooldown":        {"inventory:time-skip", "account-resources", "khan-lane:cooldown"},
		"taunt":           {"khan-lane:taunt"},
		"defense":         {"castle-focus", "defense:" + main, "khan-lane:defense"},
		"openGate":        {"castle-focus", "castle:" + main, "defense:" + main, "khan-protection", "khan-lane"},
		"protectionClear": {"khan-protection", "khan-lane"},
		"pointLimitStop":  {"khan-protection", "khan-lane", "castle:" + main},
	}
	resources := map[string][]ResourceKey{}
	for name, claims := range lanes {
		resources[name] = legacyClaimsToResources(gameState, claims)
	}
	overlaps := func(left, right string) bool {
		return resourcesOverlap(resources[left], resources[right])
	}

	// Chaining from the main castle leaves every lane free to run at once.
	for _, pair := range [][2]string{
		{"taunt", "attackFromMain"},
		{"taunt", "cooldown"},
		{"taunt", "defense"},
		{"attackFromMain", "cooldown"},
		{"attackFromMain", "defense"},
		{"cooldown", "defense"},
	} {
		if overlaps(pair[0], pair[1]) {
			t.Errorf("%s and %s claims conflict; the lanes would serialize", pair[0], pair[1])
		}
	}

	// A separate attack castle is the one case that moves session focus, so it
	// takes the focus lock the defense restock also needs.
	if !overlaps("attackFromOther", "defense") {
		t.Error("an attack from a separate castle did not take the focus lock against the defense restock")
	}
	if overlaps("attackFromOther", "taunt") {
		t.Error("a separate attack castle blocked the taunt")
	}

	for _, protection := range []string{"openGate", "protectionClear", "pointLimitStop"} {
		for _, lane := range []string{"attackFromMain", "attackFromOther", "cooldown", "taunt", "defense"} {
			if !overlaps(protection, lane) {
				t.Errorf("%s does not exclude the %s lane", protection, lane)
			}
		}
	}
}
