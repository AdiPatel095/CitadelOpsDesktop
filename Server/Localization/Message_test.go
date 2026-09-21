package Localization

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescriptorKeepsTemplateAndPrimitiveUserValues(t *testing.T) {
	message := New("test", "Send {count} troops to {name}", Params{"count": 12, "name": "Castle {admin}", "secretObject": struct{ Token string }{"private"}})
	bound := Bind(message, "Send 12 troops to Castle {admin}")
	if bound.Fallback != "Send {count} troops to {name}" || bound.FallbackText != "Send 12 troops to Castle {admin}" {
		t.Fatal(bound)
	}
	if _, ok := bound.Params["count"].(int); !ok {
		t.Fatal("numeric parameter was stringified")
	}
	raw, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "secretObject") {
		t.Fatal("nonprimitive leaked")
	}
	if message.FallbackText != "" {
		t.Fatal("mutated original")
	}
}

func TestDescriptorContextIsBoundedAndCloned(t *testing.T) {
	root := New("root", "Root", nil)
	leaf := New("leaf", "Leaf {value}", Params{"value": "original"})
	leaf.Context = []*Message{New("nested", "Nested", nil)}
	for i := 0; i < 8; i++ {
		root.Context = append(root.Context, leaf)
	}
	clone := Clone(root)
	if len(clone.Context) != 4 || len(clone.Context[0].Context) != 0 {
		t.Fatal("unbounded context")
	}
	clone.Context[0].Params["value"] = "changed"
	if leaf.Params["value"] != "original" {
		t.Fatal("mutable parameters shared")
	}
	if Join(root, nil) != nil {
		t.Fatal("masked missing reason")
	}
}

func TestGameNounParamPreservesUserContentAndSource(t *testing.T) {
	original := New("send", "Send {unit} to {castle}", Params{"unit": "Veteran Crossbowman", "castle": "Castle {admin}"})
	message := original.WithGameParam("unit", "elitecrossbowman_name", "Veteran Crossbowman")
	if original.GameParams != nil || message.GameParams["unit"].Key != "elitecrossbowman_name" || message.Params["castle"] != "Castle {admin}" {
		t.Fatal("noun metadata mutated user content or source")
	}
	if _, ok := message.GameParams["castle"]; ok {
		t.Fatal("user name marked as game text")
	}
}
