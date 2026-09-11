package Intent

import (
	"errors"
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
	var responseError *ResponseCodeError
	if !errors.As(err, &responseError) || responseError.Opcode != "mbr" ||
		responseError.Meaning.Code != 109 || responseError.Meaning.Source != GameData.ResponseCodeOfficial {
		t.Fatalf("structured response code error = %#v", responseError)
	}
}

func TestUnsuccessfulResponseCodeLabelsObservedInference(t *testing.T) {
	engine := &Engine{}

	err := engine.unsuccessfulResponseCode("hru", 53)
	if err == nil || !strings.Contains(err.Error(), "castle focus") || !strings.Contains(err.Error(), "inferred from captures") {
		t.Fatalf("response code error = %v", err)
	}
	var responseError *ResponseCodeError
	if !errors.As(err, &responseError) || !responseError.Meaning.ExpectedState ||
		responseError.Meaning.Kind != GameData.ResponseCodeContext || responseError.Meaning.Recovery == "" {
		t.Fatalf("observed response guidance = %#v", responseError)
	}
}

func TestUnsuccessfulResponseCodeLabelsOfficialClientMeaning(t *testing.T) {
	engine := &Engine{}

	err := engine.unsuccessfulResponseCode("ere", 227)
	if err == nil || !strings.Contains(err.Error(), "enchantment attempt failed") ||
		!strings.Contains(err.Error(), "official game client") {
		t.Fatalf("response code error = %v", err)
	}
	var responseError *ResponseCodeError
	if !errors.As(err, &responseError) || responseError.Opcode != "ere" ||
		responseError.Meaning.Source != GameData.ResponseCodeOfficialClient ||
		!responseError.Meaning.ExpectedState || responseError.Meaning.Recovery == "" {
		t.Fatalf("official-client response code error = %#v", responseError)
	}
}
