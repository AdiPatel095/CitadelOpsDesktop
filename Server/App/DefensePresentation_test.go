package App

import (
	"CitadelDesktop/Server/State"
	"strings"
	"testing"
)

func TestDefenseSummaryKeepsUserNamesAndUsesUnnamedIDVariant(t *testing.T) {
	for _, kind := range []string{"refresh", "gates", "wall", "moat", "keep", "preset"} {
		named := defenseSummaryDescriptor(kind, State.CastleState{ID: 42, Name: "Castle {admin}"}, "Preset {raw}")
		if named == nil || named.Params["castle"] != "Castle {admin}" || strings.Contains(named.Fallback, "admin") || !strings.HasSuffix(named.Key, ".named") {
			t.Fatalf("user name became template: %#v", named)
		}
		unnamed := defenseSummaryDescriptor(kind, State.CastleState{ID: 42}, "Preset {raw}")
		if unnamed == nil || unnamed.Params["id"] != "42" || !strings.Contains(unnamed.Fallback, "castle {id}") || !strings.HasSuffix(unnamed.Key, ".id") {
			t.Fatalf("missing ID-only variant: %#v", unnamed)
		}
		if kind == "preset" && (named.Params["preset"] != "Preset {raw}" || unnamed.Params["preset"] != "Preset {raw}") {
			t.Fatal("preset user content changed")
		}
	}
}
