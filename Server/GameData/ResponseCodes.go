package GameData

import (
	"strconv"
	"strings"
)

type ResponseCodeSource string

const (
	ResponseCodeOfficial ResponseCodeSource = "official game text"
	ResponseCodeObserved ResponseCodeSource = "inferred from captures"
	ResponseCodeUnknown  ResponseCodeSource = "undocumented"
)

type ResponseCodeMeaning struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Source  ResponseCodeSource `json:"source"`
}

var observedResponseCodes = map[int]ResponseCodeMeaning{
	53: {
		Code:    53,
		Message: "The command was rejected because its required game context was unavailable or had changed; refresh that context and retry.",
		Source:  ResponseCodeObserved,
	},
}

var observedOpcodeResponseCodes = map[string]map[int]ResponseCodeMeaning{
	"cra": {
		256: {
			Code:    256,
			Message: "The selected commander is already assigned to an active movement or otherwise unavailable at launch time.",
			Source:  ResponseCodeObserved,
		},
	},
	"msk": {
		182: {
			Code:    182,
			Message: "The kingdom transport is no longer available to skip; refresh transport state before retrying.",
			Source:  ResponseCodeObserved,
		},
	},
	"rae": {
		327: {
			Code:    327,
			Message: "The selected fortification currency is not available for the active invasion event.",
			Source:  ResponseCodeObserved,
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
	if message, found := store.ResponseCode(code); found {
		return ResponseCodeMeaning{
			Code:    code,
			Message: message,
			Source:  ResponseCodeOfficial,
		}
	}
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if meaning, found := observedOpcodeResponseCodes[opcode][code]; found {
		return meaning
	}
	if meaning, found := observedResponseCodes[code]; found {
		if code == 53 {
			if _, focused := observedFocusedResponseOpcodes[opcode]; focused {
				meaning.Message = "The castle-scoped command was rejected because castle focus was unavailable or had been displaced; refocus the castle and retry."
			}
		}
		return meaning
	}
	return ResponseCodeMeaning{
		Code:    code,
		Message: "The game does not provide a known description for this response code.",
		Source:  ResponseCodeUnknown,
	}
}
