package GameData

import (
	"CitadelDesktop/Server/Localization"
	"strconv"
	"strings"
)

type ResponseCodeSource string

type ResponseCodeKind string

const (
	ResponseCodeOfficial       ResponseCodeSource = "official game text"
	ResponseCodeOfficialClient ResponseCodeSource = "official game client"
	ResponseCodeObserved       ResponseCodeSource = "inferred from captures"
	ResponseCodeUnknown        ResponseCodeSource = "undocumented"

	ResponseCodeAvailability ResponseCodeKind = "availability"
	ResponseCodeCooldown     ResponseCodeKind = "cooldown"
	ResponseCodeContext      ResponseCodeKind = "context"
	ResponseCodeStaleState   ResponseCodeKind = "stale_state"
)

type ResponseCodeMeaning struct {
	MessageDescriptor  *Localization.Message `json:"messageDescriptor,omitempty"`
	RecoveryDescriptor *Localization.Message `json:"recoveryDescriptor,omitempty"`
	Code               int                   `json:"code"`
	Message            string                `json:"message"`
	Source             ResponseCodeSource    `json:"source"`
	Kind               ResponseCodeKind      `json:"kind,omitempty"`
	Recovery           string                `json:"recovery,omitempty"`
	ExpectedState      bool                  `json:"expectedState,omitempty"`
}

var observedResponseCodes = map[int]ResponseCodeMeaning{
	53: {
		Code:               53,
		Message:            "The command was rejected because its required game context was unavailable or had changed.",
		MessageDescriptor:  Localization.New("server.gamedata.the_command_was_rejected.86af9cd2", "The command was rejected because its required game context was unavailable or had changed.", nil),
		Source:             ResponseCodeObserved,
		Kind:               ResponseCodeContext,
		Recovery:           "Refresh the affected feature and retry after its current game context is restored.",
		RecoveryDescriptor: Localization.New("server.gamedata.refresh_the_affected_feature.441ed05e", "Refresh the affected feature and retry after its current game context is restored.", nil),
		ExpectedState:      true,
	},
}

var officialClientEnchantResponseCodes = map[int]ResponseCodeMeaning{
	226: {
		Code:               226,
		Message:            "The item's enchantment level is too high for another enchantment attempt.",
		MessageDescriptor:  Localization.New("server.gamedata.the_item_s_enchantment.04fceee7", "The item's enchantment level is too high for another enchantment attempt.", nil),
		Source:             ResponseCodeOfficialClient,
		Kind:               ResponseCodeStaleState,
		Recovery:           "Refresh equipment and select an item below its maximum enchantment level.",
		RecoveryDescriptor: Localization.New("server.gamedata.refresh_equipment_and_select.5b2cc553", "Refresh equipment and select an item below its maximum enchantment level.", nil),
		ExpectedState:      true,
	},
	227: {
		Code:               227,
		Message:            "The enchantment attempt failed, so the item did not gain a level.",
		MessageDescriptor:  Localization.New("server.gamedata.the_enchantment_attempt_failed.f8de0dd5", "The enchantment attempt failed, so the item did not gain a level.", nil),
		Source:             ResponseCodeOfficialClient,
		Recovery:           "Retry the same level after rechecking the remaining coins and, for relic upgrades, relic splinters.",
		RecoveryDescriptor: Localization.New("server.gamedata.retry_the_same_level.a2d7073b", "Retry the same level after rechecking the remaining coins and, for relic upgrades, relic splinters.", nil),
		ExpectedState:      true,
	},
	236: {
		Code:               236,
		Message:            "The selected item cannot be enchanted.",
		MessageDescriptor:  Localization.New("server.gamedata.the_selected_item_cannot.90b1b05a", "The selected item cannot be enchanted.", nil),
		Source:             ResponseCodeOfficialClient,
		Kind:               ResponseCodeContext,
		Recovery:           "Refresh equipment and select an item the game currently allows to be enchanted.",
		RecoveryDescriptor: Localization.New("server.gamedata.refresh_equipment_and_select.2160a59c", "Refresh equipment and select an item the game currently allows to be enchanted.", nil),
		ExpectedState:      true,
	},
}

var officialClientOpcodeResponseCodes = map[string]map[int]ResponseCodeMeaning{
	// Official client C2_CONFIRMATION_REQUIRED. Scoped to the verified EUP flow.
	"eup": {440: {Code: 440, Message: "This ruby purchase requires confirmation in the game.", MessageDescriptor: Localization.New("server.gamedata.ruby_purchase_confirmation", "This ruby purchase requires confirmation in the game.", nil), Source: ResponseCodeOfficialClient}},
	// Official client enum: NO_MULTIPLE_ALLIANCEHELP = 273.
	// https://empire-html5.goodgamestudios.com/default/dll/ggs.dll.6644f9217d73e8ce169d.js
	"ahr": {
		273: {
			Code:               273,
			Message:            "The alliance-help request was rejected as a duplicate or multiple request.",
			MessageDescriptor:  Localization.New("server.gamedata.the_alliance_help_request.50f10082", "The alliance-help request was rejected as a duplicate or multiple request.", nil),
			Source:             ResponseCodeOfficialClient,
			Kind:               ResponseCodeStaleState,
			Recovery:           "Wait for the existing alliance-help request to complete or refresh its state before requesting help again.",
			RecoveryDescriptor: Localization.New("server.gamedata.wait_for_the_existing.7385cb30", "Wait for the existing alliance-help request to complete or refresh its state before requesting help again.", nil),
			ExpectedState:      true,
		},
	},
	// Official client enum: NO_FREE_CONSTRUCTION_ITEM_SLOT = 374.
	// The server can retain an expired temporary item as attached after its
	// effect timer reaches zero, so RPC callers must refresh and wait for a
	// snapshot that actually removes it before equipping a replacement.
	"rpc": {
		374: {
			Code:               374,
			Message:            "The selected building has no free construction-item slot.",
			MessageDescriptor:  Localization.New("server.gamedata.the_selected_building_has.e43d0088", "The selected building has no free construction-item slot.", nil),
			Source:             ResponseCodeOfficialClient,
			Kind:               ResponseCodeStaleState,
			Recovery:           "Refresh the castle's construction-item slots and wait until the attached item is removed before equipping another one.",
			RecoveryDescriptor: Localization.New("server.gamedata.refresh_the_castle_s.38ac48d8", "Refresh the castle's construction-item slots and wait until the attached item is removed before equipping another one.", nil),
			ExpectedState:      true,
		},
	},
	"ere": officialClientEnchantResponseCodes,
	"eqe": officialClientEnchantResponseCodes,
}

var observedOpcodeResponseCodes = map[string]map[int]ResponseCodeMeaning{
	"cra": {
		91: {
			Code:               91,
			Message:            "The selected attack preset has incompatible tools assigned for this attack.",
			MessageDescriptor:  Localization.New("server.gamedata.the_selected_attack_preset.dcb72753", "The selected attack preset has incompatible tools assigned for this attack.", nil),
			Source:             ResponseCodeObserved,
			Kind:               ResponseCodeContext,
			Recovery:           "Remove or replace the incompatible tools in the selected attack preset, then retry.",
			RecoveryDescriptor: Localization.New("server.gamedata.remove_or_replace_the.a06a9d3d", "Remove or replace the incompatible tools in the selected attack preset, then retry.", nil),
		},
		256: {
			Code:               256,
			Message:            "The selected commander is already assigned to an active movement or otherwise unavailable at launch time.",
			MessageDescriptor:  Localization.New("server.gamedata.the_selected_commander_is.3de439ed", "The selected commander is already assigned to an active movement or otherwise unavailable at launch time.", nil),
			Source:             ResponseCodeObserved,
			Kind:               ResponseCodeAvailability,
			Recovery:           "Wait for a commander to return. Automated combat pauses after this response to avoid repeated rejected launches.",
			RecoveryDescriptor: Localization.New("server.gamedata.wait_for_a_commander.b2b9dfc8", "Wait for a commander to return. Automated combat pauses after this response to avoid repeated rejected launches.", nil),
			ExpectedState:      true,
		},
	},
	"msk": {
		182: {
			Code:               182,
			Message:            "The kingdom transport is no longer available to skip.",
			MessageDescriptor:  Localization.New("server.gamedata.the_kingdom_transport_is.c82d73f7", "The kingdom transport is no longer available to skip.", nil),
			Source:             ResponseCodeObserved,
			Kind:               ResponseCodeStaleState,
			Recovery:           "Refresh kingdom transport state before selecting another time skip.",
			RecoveryDescriptor: Localization.New("server.gamedata.refresh_kingdom_transport_state.fdc2cbd4", "Refresh kingdom transport state before selecting another time skip.", nil),
			ExpectedState:      true,
		},
	},
	"rae": {
		327: {
			Code:               327,
			Message:            "The selected fortification currency is not available for the active invasion event.",
			MessageDescriptor:  Localization.New("server.gamedata.the_selected_fortification_currency.3efda80f", "The selected fortification currency is not available for the active invasion event.", nil),
			Source:             ResponseCodeObserved,
			Kind:               ResponseCodeAvailability,
			Recovery:           "Refresh the event and choose one of the currencies it currently offers.",
			RecoveryDescriptor: Localization.New("server.gamedata.refresh_the_event_and.af52b44e", "Refresh the event and choose one of the currencies it currently offers.", nil),
			ExpectedState:      true,
		},
	},
}

type responseCodeGuidance struct {
	recoveryDescriptor *Localization.Message
	kind               ResponseCodeKind
	recovery           string
	expectedState      bool
}

var responseCodeGuidanceByCode = map[int]responseCodeGuidance{
	147: {
		kind: ResponseCodeStaleState, expectedState: true,
		recovery:           "Refresh the feature before retrying; the requested process may already be complete.",
		recoveryDescriptor: Localization.New("server.gamedata.refresh_the_feature_before.991e81cb", "Refresh the feature before retrying; the requested process may already be complete.", nil),
	},
	175: {
		kind: ResponseCodeContext, expectedState: true,
		recovery:           "Choose a location in a kingdom this account can currently access, then refresh the feature.",
		recoveryDescriptor: Localization.New("server.gamedata.choose_a_location_in.fbb11a63", "Choose a location in a kingdom this account can currently access, then refresh the feature.", nil),
	},
}

var responseCodeGuidanceByOpcode = map[string]map[int]responseCodeGuidance{
	"adi": {
		95: {
			kind: ResponseCodeCooldown, expectedState: true,
			recovery:           "Wait for the target cooldown to end, or refresh the world map before choosing another target.",
			recoveryDescriptor: Localization.New("server.gamedata.wait_for_the_target.38495259", "Wait for the target cooldown to end, or refresh the world map before choosing another target.", nil),
		},
	},
	"bup": {
		87: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery:           "Refresh recruitable troops and the castle's production buildings before choosing another troop.",
			recoveryDescriptor: Localization.New("server.gamedata.refresh_recruitable_troops_and.ff19841e", "Refresh recruitable troops and the castle's production buildings before choosing another troop.", nil),
		},
	},
	"cds": {
		101: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery:           "Refresh the troop selection before trying again.",
			recoveryDescriptor: Localization.New("server.gamedata.refresh_the_troop_selection.57aaa1ca", "Refresh the troop selection before trying again.", nil),
		},
	},
	"ere": {
		222: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery:           "Wait for the commander or castellan carrying this item to return, then refresh equipment before retrying.",
			recoveryDescriptor: Localization.New("server.gamedata.wait_for_the_commander.851b76dd", "Wait for the commander or castellan carrying this item to return, then refresh equipment before retrying.", nil),
		},
	},
	"eqe": {
		222: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery:           "Wait for the commander or castellan carrying this item to return, then refresh equipment before retrying.",
			recoveryDescriptor: Localization.New("server.gamedata.wait_for_the_commander.851b76dd", "Wait for the commander or castellan carrying this item to return, then refresh equipment before retrying.", nil),
		},
	},
	"ebe": {
		263: {
			kind: ResponseCodeContext, expectedState: true,
			recovery:           "Choose a different expansion direction, then refresh the castle before retrying.",
			recoveryDescriptor: Localization.New("server.gamedata.choose_a_different_expansion.65d33eb2", "Choose a different expansion direction, then refresh the castle before retrying.", nil),
		},
	},
	"jaa": {
		337: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery:           "Unlock or enter this kingdom in the game, then refresh the feature.",
			recoveryDescriptor: Localization.New("server.gamedata.unlock_or_enter_this.4bcb6aa8", "Unlock or enter this kingdom in the game, then refresh the feature.", nil),
		},
	},
	"sbp": {
		55: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery:           "Wait for enough shop currency or lower the purchase amount, then refresh the shop.",
			recoveryDescriptor: Localization.New("server.gamedata.wait_for_enough_shop.fc9d2343", "Wait for enough shop currency or lower the purchase amount, then refresh the shop.", nil),
		},
		159: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery:           "Refresh the shop before choosing an available offer again.",
			recoveryDescriptor: Localization.New("server.gamedata.refresh_the_shop_before.7311cea6", "Refresh the shop before choosing an available offer again.", nil),
		},
		203: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery:           "Refresh the shop before choosing an available offer again.",
			recoveryDescriptor: Localization.New("server.gamedata.refresh_the_shop_before.7311cea6", "Refresh the shop before choosing an available offer again.", nil),
		},
	},
}

var observedFocusedResponseOpcodes = map[string]struct{}{
	"ahr": {},
	"gui": {},
	"hru": {},
	"spl": {},
}

func (store *LanguageStore) ResponseCode(code int) (string, bool) {
	return store.Resolve("errorCode_" + strconv.Itoa(code))
}

// ResponseCodes returns a copy of every official errorCode_<number> entry in
// the currently loaded game language catalog.
func (store *LanguageStore) ResponseCodes() map[int]string {
	codes := make(map[int]string)
	if store == nil {
		return codes
	}
	for key, message := range store.values {
		if !strings.HasPrefix(key, "errorCode_") || strings.TrimSpace(message) == "" {
			continue
		}
		code, err := strconv.Atoi(strings.TrimPrefix(key, "errorCode_"))
		if err == nil {
			codes[code] = message
		}
	}
	return codes
}

// ResponseCodeMeanings combines the official language catalog with meanings
// published in the official game client and inferred from captures when the
// game does not publish language text for a code.
func (store *LanguageStore) ResponseCodeMeanings(opcode string) map[int]ResponseCodeMeaning {
	meanings := make(map[int]ResponseCodeMeaning)
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	for code := range store.ResponseCodes() {
		meanings[code] = ResolveResponseCode(store, opcode, code)
	}
	for code := range officialClientOpcodeResponseCodes[opcode] {
		if _, found := meanings[code]; !found {
			meanings[code] = ResolveResponseCode(store, opcode, code)
		}
	}
	for code := range observedResponseCodes {
		if _, found := meanings[code]; !found {
			meanings[code] = ResolveResponseCode(store, opcode, code)
		}
	}
	for code := range observedOpcodeResponseCodes[opcode] {
		if _, found := meanings[code]; !found {
			meanings[code] = ResolveResponseCode(store, opcode, code)
		}
	}
	return meanings
}

func ResolveResponseCode(store *LanguageStore, opcode string, code int) ResponseCodeMeaning {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	var meaning ResponseCodeMeaning
	if message, found := store.ResponseCode(code); found {
		meaning = ResponseCodeMeaning{
			Code: code, Message: message, Source: ResponseCodeOfficial,
			MessageDescriptor: Localization.Official("errorCode_"+strconv.Itoa(code), message),
		}
	} else if officialClient, found := officialClientOpcodeResponseCodes[opcode][code]; found {
		meaning = officialClient
	} else if observed, found := observedOpcodeResponseCodes[opcode][code]; found {
		meaning = observed
	} else if observed, found := observedResponseCodes[code]; found {
		meaning = observed
		if code == 53 {
			if _, focused := observedFocusedResponseOpcodes[opcode]; focused {
				meaning.Message = "The castle-scoped command was rejected because castle focus was unavailable or had been displaced."
				meaning.MessageDescriptor = Localization.New("server.gamedata.the_castle_scoped_command.ab69acc8", "The castle-scoped command was rejected because castle focus was unavailable or had been displaced.", nil)
				meaning.Recovery = "Let the app restore castle focus before retrying."
				meaning.RecoveryDescriptor = Localization.New("server.gamedata.let_the_app_restore.70312eef", "Let the app restore castle focus before retrying.", nil)
			}
		}
	} else {
		meaning = ResponseCodeMeaning{
			Code:              code,
			Message:           "The game does not provide a known description for this response code.",
			MessageDescriptor: Localization.New("server.gamedata.the_game_does_not.00a274ac", "The game does not provide a known description for this response code.", nil),
			Source:            ResponseCodeUnknown,
		}
	}
	if meaning.Source == ResponseCodeOfficial {
		if officialClient, found := officialClientOpcodeResponseCodes[opcode][code]; found {
			meaning.Kind = officialClient.Kind
			meaning.Recovery = officialClient.Recovery
			meaning.RecoveryDescriptor = officialClient.RecoveryDescriptor
			meaning.ExpectedState = officialClient.ExpectedState
		} else if observed, found := observedOpcodeResponseCodes[opcode][code]; found {
			meaning.Kind = observed.Kind
			meaning.Recovery = observed.Recovery
			meaning.RecoveryDescriptor = observed.RecoveryDescriptor
			meaning.ExpectedState = observed.ExpectedState
		} else if observed, found := observedResponseCodes[code]; found {
			meaning.Kind = observed.Kind
			meaning.Recovery = observed.Recovery
			meaning.RecoveryDescriptor = observed.RecoveryDescriptor
			meaning.ExpectedState = observed.ExpectedState
			if code == 53 {
				if _, focused := observedFocusedResponseOpcodes[opcode]; focused {
					meaning.Recovery = "Let the app restore castle focus before retrying."
					meaning.RecoveryDescriptor = Localization.New("server.gamedata.let_the_app_restore.70312eef", "Let the app restore castle focus before retrying.", nil)
				}
			}
		}
	}
	if guidance, found := responseCodeGuidanceByCode[code]; found {
		meaning = applyResponseCodeGuidance(meaning, guidance)
	}
	if guidance, found := responseCodeGuidanceByOpcode[opcode][code]; found {
		meaning = applyResponseCodeGuidance(meaning, guidance)
	}
	meaning.MessageDescriptor = Localization.Clone(meaning.MessageDescriptor)
	meaning.RecoveryDescriptor = Localization.Clone(meaning.RecoveryDescriptor)
	return meaning
}

func applyResponseCodeGuidance(meaning ResponseCodeMeaning, guidance responseCodeGuidance) ResponseCodeMeaning {
	meaning.Kind = guidance.kind
	meaning.Recovery = guidance.recovery
	meaning.RecoveryDescriptor = guidance.recoveryDescriptor
	meaning.ExpectedState = guidance.expectedState
	meaning.MessageDescriptor = Localization.Clone(meaning.MessageDescriptor)
	meaning.RecoveryDescriptor = Localization.Clone(meaning.RecoveryDescriptor)
	return meaning
}
