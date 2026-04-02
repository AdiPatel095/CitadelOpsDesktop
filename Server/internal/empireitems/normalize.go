package empireitems

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	reCamelSplit   = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	reLetterDigit  = regexp.MustCompile(`([A-Za-z])(\d)`)
	reDigitLetter  = regexp.MustCompile(`(\d)([A-Za-z])`)
	reAcronymSplit = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	reOfSpace      = regexp.MustCompile(`(?i)([A-Za-z]{2,})of(\s+)`)
	reOfEnd        = regexp.MustCompile(`(?i)([A-Za-z]{2,})of$`)
)

// FormatDisplayNameFromInternalType turns GGE internal type (e.g. Supplies1, VictoryMemorial) into a readable label.
func FormatDisplayNameFromInternalType(typ string) string {
	if typ == "" {
		return typ
	}
	s := reCamelSplit.ReplaceAllString(typ, "$1 $2")
	s = reLetterDigit.ReplaceAllString(s, "$1 $2")
	s = reDigitLetter.ReplaceAllString(s, "$1 $2")
	s = reAcronymSplit.ReplaceAllString(s, "$1 $2")
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return typ
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if isAllDigits(p) {
			out = append(out, p)
		} else if len(p) == 1 {
			out = append(out, strings.ToUpper(p))
		} else {
			r := []rune(p)
			out = append(out, string(unicode.ToUpper(r[0]))+strings.ToLower(string(r[1:])))
		}
	}
	return strings.Join(out, " ")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// RefineDecoParticleOf splits stuck "wordof Word" into "Word Of Word". Idempotent on already-fixed names.
func RefineDecoParticleOf(name string) string {
	if name == "" {
		return name
	}
	out := name
	for {
		prev := out
		out = reOfSpace.ReplaceAllString(out, "${1} Of${2}")
		out = reOfEnd.ReplaceAllString(out, "${1} Of")
		if prev == out {
			break
		}
	}
	return out
}

// NormalizeDecoBuildings mutates rows where name is "Deco" (any case): sets type to "deco" and name to a display label from the former type.
func NormalizeDecoBuildings(buildings []map[string]any) {
	for _, b := range buildings {
		if b == nil {
			continue
		}
		nm, _ := b["name"].(string)
		if strings.TrimSpace(strings.ToLower(nm)) != "deco" {
			continue
		}
		oldType, _ := b["type"].(string)
		b["type"] = "deco"
		b["name"] = RefineDecoParticleOf(FormatDisplayNameFromInternalType(oldType))
	}
}
