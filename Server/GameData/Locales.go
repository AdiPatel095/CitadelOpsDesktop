package GameData

import (
	"fmt"
	"strings"
)

// Official locales verified against the live game bundle and language service
// version 4357 on 2026-09-20. Lithuanian is served but absent from the alliance
// language picker. The Chinese service codes are case-sensitive and differ
// from the lowercase alliance picker aliases.
const LocaleManifestVersion = "4357"
const LocaleManifestSource = "https://langserv.public.ggs-ep.com/12@4357/en/*"

type Locale struct {
	Code       string `json:"code"`
	GameCode   string `json:"gameCode"`
	NativeName string `json:"nativeName"`
	Name       string `json:"name"`
	Direction  string `json:"direction"`
}

var officialLocales = []Locale{
	{"en", "en", "English", "English", "ltr"},
	{"de", "de", "Deutsch", "German", "ltr"},
	{"fr", "fr", "Français", "French", "ltr"},
	{"pl", "pl", "Polski", "Polish", "ltr"},
	{"ru", "ru", "Русский", "Russian", "ltr"},
	{"it", "it", "Italiano", "Italian", "ltr"},
	{"nl", "nl", "Nederlands", "Dutch", "ltr"},
	{"pt", "pt", "Português", "Portuguese", "ltr"},
	{"es", "es", "Español", "Spanish", "ltr"},
	{"ar", "ar", "العربية", "Arabic", "rtl"},
	{"da", "da", "Dansk", "Danish", "ltr"},
	{"no", "no", "Norsk", "Norwegian", "ltr"},
	{"fi", "fi", "Suomi", "Finnish", "ltr"},
	{"sv", "sv", "Svenska", "Swedish", "ltr"},
	{"ja", "ja", "日本語", "Japanese", "ltr"},
	{"ko", "ko", "한국어", "Korean", "ltr"},
	{"el", "el", "Ελληνικά", "Greek", "ltr"},
	{"tr", "tr", "Türkçe", "Turkish", "ltr"},
	{"zh-CN", "zh_CN", "中文(简体)", "Chinese (Simplified)", "ltr"},
	{"zh-TW", "zh_TW", "中文(繁體)", "Chinese (Traditional)", "ltr"},
	{"cs", "cs", "Čeština", "Czech", "ltr"},
	{"ro", "ro", "Română", "Romanian", "ltr"},
	{"sk", "sk", "Slovenčina", "Slovak", "ltr"},
	{"hu", "hu", "Magyar", "Hungarian", "ltr"},
	{"bg", "bg", "български", "Bulgarian", "ltr"},
	{"lt", "lt", "Lietuvių", "Lithuanian", "ltr"},
}

func OfficialLocales() []Locale { return append([]Locale(nil), officialLocales...) }

// NormalizeLocale accepts only manifest codes and their underscore aliases.
// World/country identifiers are deliberately not language identifiers.
func NormalizeLocale(value string) (Locale, error) {
	code := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if code == "" {
		code = "en"
	}
	for _, locale := range officialLocales {
		if strings.ToLower(locale.Code) == code {
			return locale, nil
		}
	}
	return Locale{}, fmt.Errorf("unsupported locale %q", value)
}
