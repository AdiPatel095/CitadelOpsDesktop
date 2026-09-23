package Localization

import (
	"encoding/json"
	"strings"
	"testing"
)

func listFixture() *Message {
	return WithLists(New("root", "Buy {items}", nil), "Buy literal original", map[string][]*Message{"items": {New("leaf", "Package {id}", Params{"id": "123"})}})
}
func TestListBoundsAndWholeFallback(t *testing.T) {
	cases := map[string]func(*Message){
		"binding":        func(m *Message) { m.FallbackText = "" },
		"official":       func(m *Message) { m.OfficialKey = "bad" },
		"empty":          func(m *Message) { m.ListParams = map[string][]*Message{} },
		"collision":      func(m *Message) { m.Params = Params{"items": "bad"} },
		"game collision": func(m *Message) { m.GameParams = map[string]GameParam{"items": {Key: "bad"}} },
		"reserved":       func(m *Message) { m.ListParams["constructor"] = m.ListParams["items"] },
		"nested":         func(m *Message) { m.ListParams["items"][0].ListParams = map[string][]*Message{} },
		"context":        func(m *Message) { m.ListParams["items"][0].Context = []*Message{} },
		"nil leaf":       func(m *Message) { m.ListParams["items"][0] = nil },
		"item count": func(m *Message) {
			for len(m.ListParams["items"]) < 33 {
				m.ListParams["items"] = append(m.ListParams["items"], New("a", "A", nil))
			}
		},
		"escaped bytes": func(m *Message) { m.ListParams["items"][0].Params = Params{"id": strings.Repeat("<", 11000)} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := listFixture()
			mutate(m)
			if Validate(m) == nil || Clone(m) != nil {
				t.Fatal("invalid list survived")
			}
		})
	}
	m := listFixture()
	copy := Clone(m)
	copy.ListParams["items"][0].Params["id"] = "other"
	if m.ListParams["items"][0].Params["id"] != "123" {
		t.Fatal("clone aliases source")
	}
	data, err := json.Marshal(m)
	if err != nil || !strings.Contains(string(data), `"fallbackText":"Buy literal original"`) {
		t.Fatalf("bound wire: %s %v", data, err)
	}
	if Join(m, New("b", "B", nil)) != nil || Join(New("b", "B", nil), m) != nil {
		t.Fatal("join nested a list")
	}
}
func TestListAggregateBoundary(t *testing.T) {
	m := listFixture()
	m.ListParams = map[string][]*Message{}
	for _, name := range []string{"one", "two"} {
		for i := 0; i < 32; i++ {
			m.ListParams[name] = append(m.ListParams[name], New("leaf", "Leaf", nil))
		}
	}
	if Validate(m) != nil {
		t.Fatal("64 leaves rejected")
	}
	m.ListParams["three"] = []*Message{New("leaf", "Leaf", nil)}
	if Validate(m) == nil {
		t.Fatal("65 leaves accepted")
	}
}

func TestMalformedContextListsNeverPanic(t *testing.T) {
	bad := New("root", "Root", nil)
	bad.Context = []*Message{{Key: "leaf", Fallback: "Leaf", ListParams: map[string][]*Message{}}}
	if Clone(bad) != nil || Bind(bad, "raw") != nil || Join(bad, New("ok", "OK", nil)) != nil || Join(New("ok", "OK", nil), bad) != nil {
		t.Fatal("invalid context metadata survived")
	}
	if _, err := json.Marshal(bad); err != nil {
		t.Fatal(err)
	}
}
