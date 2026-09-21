// Package Localization carries producer-owned presentation metadata. It never
// translates user input, diagnostic strings, or durable operation evidence.
package Localization

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
)

type Params map[string]any

type GameParam struct {
	Key      string `json:"key"`
	Fallback string `json:"fallback"`
	Params   Params `json:"params,omitempty"`
}

type Message struct {
	FallbackText   string               `json:"fallbackText,omitempty"`
	Context        []*Message           `json:"context,omitempty"`
	Key            string               `json:"key"`
	Fallback       string               `json:"fallback"`
	Params         Params               `json:"params,omitempty"`
	OfficialKey    string               `json:"officialKey,omitempty"`
	OfficialParams Params               `json:"officialParams,omitempty"`
	GameParams     map[string]GameParam `json:"gameParams,omitempty"`
}

func New(key, fallback string, params Params) *Message {
	return &Message{Key: key, Fallback: fallback, Params: primitiveParams(params)}
}

func Official(key, fallback string) *Message {
	return &Message{Key: "game." + key, OfficialKey: key, Fallback: fallback}
}

// Clone retains only primitive interpolation values. Nested values cannot leak
// private runtime objects into the presentation contract.
func Clone(message *Message) *Message {
	if message == nil {
		return nil
	}
	copy := *message
	copy.Context = nil
	for i, leaf := range message.Context {
		if i >= 4 {
			break
		}
		if leaf != nil {
			v := *leaf
			v.Context = nil
			copy.Context = append(copy.Context, Clone(&v))
		}
	}
	copy.Params = primitiveParams(message.Params)
	copy.OfficialParams = primitiveParams(message.OfficialParams)
	if message.GameParams != nil {
		copy.GameParams = make(map[string]GameParam, len(message.GameParams))
		for key, value := range message.GameParams {
			value.Params = primitiveParams(value.Params)
			copy.GameParams[key] = value
		}
	}
	return &copy
}

func primitiveParams(params Params) Params {
	if len(params) == 0 {
		return nil
	}
	result := Params{}
	for key, value := range params {
		switch value := value.(type) {
		case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			result[key] = value
		case float32:
			if !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) {
				result[key] = value
			}
		case float64:
			if !math.IsNaN(value) && !math.IsInf(value, 0) {
				result[key] = value
			}
		default:
			v := reflect.ValueOf(value)
			if v.IsValid() {
				switch v.Kind() {
				case reflect.String:
					result[key] = v.String()
				case reflect.Bool:
					result[key] = v.Bool()
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					result[key] = v.Int()
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					result[key] = v.Uint()
				}
			}
		}
	}
	return result
}

// Bind changes only the display fallback after existing sanitization. Callers
// must discard a descriptor when sanitization changes its meaning.
func Bind(message *Message, fallback string) *Message {
	if message == nil {
		return nil
	}
	copy := Clone(message)
	copy.FallbackText = fallback
	return copy
}

func Status(message *Message) string {
	if message != nil && (message.Key != "" || message.OfficialKey != "") {
		return "structured"
	}
	return "untranslated"
}

// Format uses a producer-owned template and explicitly preformatted primitive
// arguments. The English template is stored in en.json for client catalogs.
func Format(key, fallback string, params Params) *Message { return New(key, fallback, params) }

func (message Message) MarshalJSON() ([]byte, error) {
	type wire Message
	return json.Marshal((*wire)(Clone(&message)))
}

func Validate(message *Message) error {
	if message == nil {
		return nil
	}
	if message.Key == "" && message.OfficialKey == "" {
		return fmt.Errorf("message has no translation key")
	}
	if len(message.Params) != len(primitiveParams(message.Params)) || len(message.OfficialParams) != len(primitiveParams(message.OfficialParams)) {
		return fmt.Errorf("message has non-primitive parameters")
	}
	for _, param := range message.GameParams {
		if len(param.Params) != len(primitiveParams(param.Params)) {
			return fmt.Errorf("game parameter has non-primitive values")
		}
	}
	return nil
}

// Join preserves both independently translatable messages. An unknown child
// cannot be replaced by a generic translation that would hide its exact reason.
func Join(first, second *Message) *Message {
	if first == nil || second == nil {
		return nil
	}
	// Context is displayed before the main message: explanation, then recovery.
	if len(first.Context)+1+len(second.Context) > 4 {
		return nil
	}
	combined := Clone(second)
	prefix := []*Message{}
	for _, leaf := range first.Context {
		v := Clone(leaf)
		v.Context = nil
		prefix = append(prefix, v)
	}
	firstLeaf := Clone(first)
	firstLeaf.Context = nil
	prefix = append(prefix, firstLeaf)
	combined.Context = append(prefix, combined.Context...)
	return combined
}

func First(messages []*Message) *Message {
	if len(messages) == 0 {
		return nil
	}
	return Clone(messages[0])
}
