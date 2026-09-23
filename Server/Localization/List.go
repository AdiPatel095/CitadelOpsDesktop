package Localization

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

// WithLists binds the complete raw root before validating bounded, leaf-only lists.
// Invalid metadata is omitted; callers retain their existing raw field unchanged.
func WithLists(message *Message, fallback string, lists map[string][]*Message) *Message {
	if message == nil {
		return nil
	}
	root := *message
	root.FallbackText = fallback
	root.ListParams = lists
	return Clone(&root)
}

func validateLists(message *Message) error {
	if message == nil {
		return nil
	}
	for _, leaf := range message.Context {
		if leaf != nil && leaf.ListParams != nil {
			return fmt.Errorf("context cannot contain lists")
		}
	}
	if message.ListParams == nil {
		return nil
	}
	if message.FallbackText == "" || message.OfficialKey != "" || len(message.ListParams) < 1 || len(message.ListParams) > 4 {
		return fmt.Errorf("invalid list root")
	}
	count := 0
	for name, leaves := range message.ListParams {
		if name == "__proto__" || name == "prototype" || name == "constructor" {
			return fmt.Errorf("reserved list name")
		}
		if _, ok := message.Params[name]; ok {
			return fmt.Errorf("list parameter collision")
		}
		if _, ok := message.GameParams[name]; ok {
			return fmt.Errorf("list game parameter collision")
		}
		count += len(leaves)
		if len(leaves) < 1 || len(leaves) > 32 || count > 64 {
			return fmt.Errorf("list size exceeded")
		}
		for _, leaf := range leaves {
			if leaf == nil || leaf.Context != nil || leaf.ListParams != nil {
				return fmt.Errorf("list item must be a leaf")
			}
			if err := validateListLeaf(leaf); err != nil {
				return err
			}
		}
	}
	data, err := json.Marshal(message.ListParams)
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return fmt.Errorf("serialized lists exceed 64KiB")
	}
	return nil
}

func validateListLeaf(m *Message) error {
	if strings.TrimSpace(m.Key) == "" || len(utf16.Encode([]rune(m.Key))) > 512 || strings.TrimSpace(m.Fallback) == "" || len(utf16.Encode([]rune(m.Fallback))) > 16384 || len(utf16.Encode([]rune(m.FallbackText))) > 16384 {
		return fmt.Errorf("invalid list leaf text")
	}
	for _, params := range []Params{m.Params, m.OfficialParams} {
		if len(params) > 64 {
			return fmt.Errorf("too many leaf parameters")
		}
		for name := range params {
			if name == "__proto__" || name == "prototype" || name == "constructor" {
				return fmt.Errorf("reserved leaf parameter")
			}
		}
	}
	if m.OfficialKey != "" && (strings.TrimSpace(m.OfficialKey) == "" || len(utf16.Encode([]rune(m.OfficialKey))) > 512) {
		return fmt.Errorf("invalid official key")
	}
	if m.OfficialParams != nil && m.OfficialKey == "" {
		return fmt.Errorf("official params without key")
	}
	if len(m.GameParams) > 64 {
		return fmt.Errorf("too many game parameters")
	}
	for name, noun := range m.GameParams {
		if name == "__proto__" || name == "prototype" || name == "constructor" {
			return fmt.Errorf("reserved game parameter")
		}
		if err := validateListLeaf(&Message{Key: noun.Key, Fallback: noun.Fallback, Params: noun.Params}); err != nil {
			return err
		}
	}
	return Validate(m)
}
