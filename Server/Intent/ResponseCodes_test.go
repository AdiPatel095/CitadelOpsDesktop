package Intent

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
)

type responseCodeTestProvider struct {
	language *GameData.LanguageStore
}

func (*responseCodeTestProvider) Current() (*GameData.Store, bool) {
	return nil, false
}

func (provider *responseCodeTestProvider) Language() (*GameData.LanguageStore, bool) {
	return provider.language, provider.language != nil
}

func TestUnsuccessfulResponseCodeUsesLoadedLanguageCatalog(t *testing.T) {
	language, err := GameData.DecodeLanguage(
		[]byte(`{"errorCode_109":"All market barrows are moving."}`),
		GameData.LanguageMetadata{Language: "en"},
	)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{gameData: &responseCodeTestProvider{language: language}}

	err = engine.unsuccessfulResponseCode("mbr", 109)
	if err == nil || !strings.Contains(err.Error(), "All market barrows are moving.") || !strings.Contains(err.Error(), "official game text") {
		t.Fatalf("response code error = %v", err)
	}
}

func TestUnsuccessfulResponseCodeLabelsObservedInference(t *testing.T) {
	engine := &Engine{}

	err := engine.unsuccessfulResponseCode("hru", 53)
	if err == nil || !strings.Contains(err.Error(), "castle focus") || !strings.Contains(err.Error(), "inferred from captures") {
		t.Fatalf("response code error = %v", err)
	}
}
