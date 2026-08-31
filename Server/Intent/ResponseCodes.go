package Intent

import (
	"fmt"
	"strings"

	"CitadelDesktop/Server/GameData"
)

type responseCodeLanguageProvider interface {
	Language() (*GameData.LanguageStore, bool)
}

type ResponseCodeError struct {
	Opcode  string
	Meaning GameData.ResponseCodeMeaning
}

func (response *ResponseCodeError) Error() string {
	if response == nil {
		return "the game returned an unsuccessful response"
	}
	opcode := strings.ToUpper(strings.TrimSpace(response.Opcode))
	if opcode == "" {
		return fmt.Sprintf(
			"response code %d was not successful: %s (%s)",
			response.Meaning.Code, response.Meaning.Message, response.Meaning.Source,
		)
	}
	return fmt.Sprintf(
		"response code %d for %s was not successful: %s (%s)",
		response.Meaning.Code, opcode, response.Meaning.Message, response.Meaning.Source,
	)
}

func NewResponseCodeError(language *GameData.LanguageStore, opcode string, code int) *ResponseCodeError {
	return &ResponseCodeError{
		Opcode:  strings.ToLower(strings.TrimSpace(opcode)),
		Meaning: GameData.ResolveResponseCode(language, opcode, code),
	}
}

func (engine *Engine) unsuccessfulResponseCode(opcode string, code int) error {
	var language *GameData.LanguageStore
	if provider, ok := engine.gameData.(responseCodeLanguageProvider); ok {
		language, _ = provider.Language()
	}
	return NewResponseCodeError(language, opcode, code)
}
