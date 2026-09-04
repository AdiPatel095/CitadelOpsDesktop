package GameData

import (
	"strconv"
	"strings"
)

type ResponseCodeSource string

type ResponseCodeKind string

const (
	ResponseCodeOfficial ResponseCodeSource = "official game text"
	ResponseCodeObserved ResponseCodeSource = "inferred from captures"
	ResponseCodeUnknown  ResponseCodeSource = "undocumented"

	ResponseCodeAvailability ResponseCodeKind = "availability"
	ResponseCodeCooldown     ResponseCodeKind = "cooldown"
	ResponseCodeContext      ResponseCodeKind = "context"
	ResponseCodeStaleState   ResponseCodeKind = "stale_state"
)

type ResponseCodeMeaning struct {
	Code          int                `json:"code"`
	Message       string             `json:"message"`
	Source        ResponseCodeSource `json:"source"`
	Kind          ResponseCodeKind   `json:"kind,omitempty"`
	Recovery      string             `json:"recovery,omitempty"`
	ExpectedState bool               `json:"expectedState,omitempty"`
}

var observedResponseCodes = map[int]ResponseCodeMeaning{
	53: {
		Code:          53,
		Message:       "The command was rejected because its required game context was unavailable or had changed.",
		Source:        ResponseCodeObserved,
		Kind:          ResponseCodeContext,
		Recovery:      "Refresh the affected feature and retry after its current game context is restored.",
		ExpectedState: true,
	},
}

var observedOpcodeResponseCodes = map[string]map[int]ResponseCodeMeaning{
	"cra": {
		91: {
			Code:     91,
			Message:  "The selected attack preset has incompatible tools assigned for this attack.",
			Source:   ResponseCodeObserved,
			Kind:     ResponseCodeContext,
			Recovery: "Remove or replace the incompatible tools in the selected attack preset, then retry.",
		},
		256: {
			Code:          256,
			Message:       "The selected commander is already assigned to an active movement or otherwise unavailable at launch time.",
			Source:        ResponseCodeObserved,
			Kind:          ResponseCodeAvailability,
			Recovery:      "Wait for a commander to return. Automated combat pauses after this response to avoid repeated rejected launches.",
			ExpectedState: true,
		},
	},
	"msk": {
		182: {
			Code:          182,
			Message:       "The kingdom transport is no longer available to skip.",
			Source:        ResponseCodeObserved,
			Kind:          ResponseCodeStaleState,
			Recovery:      "Refresh kingdom transport state before selecting another time skip.",
			ExpectedState: true,
		},
	},
	"rae": {
		327: {
			Code:          327,
			Message:       "The selected fortification currency is not available for the active invasion event.",
			Source:        ResponseCodeObserved,
			Kind:          ResponseCodeAvailability,
			Recovery:      "Refresh the event and choose one of the currencies it currently offers.",
			ExpectedState: true,
		},
	},
}

type responseCodeGuidance struct {
	kind          ResponseCodeKind
	recovery      string
	expectedState bool
}

var responseCodeGuidanceByCode = map[int]responseCodeGuidance{
	147: {
		kind: ResponseCodeStaleState, expectedState: true,
		recovery: "Refresh the feature before retrying; the requested process may already be complete.",
	},
	175: {
		kind: ResponseCodeContext, expectedState: true,
		recovery: "Choose a location in a kingdom this account can currently access, then refresh the feature.",
	},
}

var responseCodeGuidanceByOpcode = map[string]map[int]responseCodeGuidance{
	"adi": {
		95: {
			kind: ResponseCodeCooldown, expectedState: true,
			recovery: "Wait for the target cooldown to end, or refresh the world map before choosing another target.",
		},
	},
	"bup": {
		87: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery: "Refresh recruitable troops and the castle's production buildings before choosing another troop.",
		},
	},
	"cds": {
		101: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery: "Refresh the troop selection before trying again.",
		},
	},
	"jaa": {
		337: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery: "Unlock or enter this kingdom in the game, then refresh the feature.",
		},
	},
	"sbp": {
		55: {
			kind: ResponseCodeAvailability, expectedState: true,
			recovery: "Wait for enough shop currency or lower the purchase amount, then refresh the shop.",
		},
		159: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery: "Refresh the shop before choosing an available offer again.",
		},
		203: {
			kind: ResponseCodeStaleState, expectedState: true,
			recovery: "Refresh the shop before choosing an available offer again.",
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
// inferred from captures when the game does not publish text for a code.
func (store *LanguageStore) ResponseCodeMeanings(opcode string) map[int]ResponseCodeMeaning {
	meanings := make(map[int]ResponseCodeMeaning)
	for code := range store.ResponseCodes() {
		meanings[code] = ResolveResponseCode(store, opcode, code)
	}
	for code := range observedResponseCodes {
		if _, found := meanings[code]; !found {
			meanings[code] = ResolveResponseCode(store, opcode, code)
		}
	}
	for code := range observedOpcodeResponseCodes[strings.ToLower(strings.TrimSpace(opcode))] {
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
			Code:    code,
			Message: message,
			Source:  ResponseCodeOfficial,
		}
	} else if observed, found := observedOpcodeResponseCodes[opcode][code]; found {
		meaning = observed
	} else if observed, found := observedResponseCodes[code]; found {
		meaning = observed
		if code == 53 {
			if _, focused := observedFocusedResponseOpcodes[opcode]; focused {
				meaning.Message = "The castle-scoped command was rejected because castle focus was unavailable or had been displaced."
				meaning.Recovery = "Let the app restore castle focus before retrying."
			}
		}
	} else {
		meaning = ResponseCodeMeaning{
			Code:    code,
			Message: "The game does not provide a known description for this response code.",
			Source:  ResponseCodeUnknown,
		}
	}
	if meaning.Source == ResponseCodeOfficial {
		if observed, found := observedOpcodeResponseCodes[opcode][code]; found {
			meaning.Kind = observed.Kind
			meaning.Recovery = observed.Recovery
			meaning.ExpectedState = observed.ExpectedState
		} else if observed, found := observedResponseCodes[code]; found {
			meaning.Kind = observed.Kind
			meaning.Recovery = observed.Recovery
			meaning.ExpectedState = observed.ExpectedState
			if code == 53 {
				if _, focused := observedFocusedResponseOpcodes[opcode]; focused {
					meaning.Recovery = "Let the app restore castle focus before retrying."
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
	return meaning
}

func applyResponseCodeGuidance(meaning ResponseCodeMeaning, guidance responseCodeGuidance) ResponseCodeMeaning {
	meaning.Kind = guidance.kind
	meaning.Recovery = guidance.recovery
	meaning.ExpectedState = guidance.expectedState
	return meaning
}
