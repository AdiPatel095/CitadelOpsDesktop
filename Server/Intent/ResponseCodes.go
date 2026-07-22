package Intent

import (
	"fmt"
	"strings"

	"CitadelDesktop/Server/GameData"
)

type responseCodeLanguageProvider interface {
	Language() (*GameData.LanguageStore, bool)
}

func (engine *Engine) unsuccessfulResponseCode(opcode string, code int) error {
	var language *GameData.LanguageStore
	if provider, ok := engine.gameData.(responseCodeLanguageProvider); ok {
		language, _ = provider.Language()
	}
	meaning := GameData.ResolveResponseCode(language, opcode, code)
	opcode = strings.ToUpper(strings.TrimSpace(opcode))
	if opcode == "" {
		return fmt.Errorf("response code %d was not successful: %s (%s)", code, meaning.Message, meaning.Source)
	}
	return fmt.Errorf("response code %d for %s was not successful: %s (%s)", code, opcode, meaning.Message, meaning.Source)
}
