// Package contractfill generates deterministic, synthetic wire-contract samples.
// It is used only by contract tests; no account or runtime data is read.
package contractfill

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})
var rawType = reflect.TypeOf(json.RawMessage{})

// Fill sets every exported JSON field. Overrides are keyed by JSON paths (with
// [] for slice elements and .* for map values), or by a JSON field name.
func Fill(value any, overrides map[string]any) error {
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("Fill needs a non-nil pointer")
	}
	n := int64(0)
	return fill(v.Elem(), "", "value", overrides, &n)
}

func fill(v reflect.Value, path, name string, overrides map[string]any, n *int64) error {
	replacement, ok := overrides[path]
	if !ok {
		replacement, ok = overrides[name]
	}
	if ok {
		r := reflect.ValueOf(replacement)
		if !r.IsValid() || !r.Type().AssignableTo(v.Type()) {
			return fmt.Errorf("override %s is not assignable to %s", path, v.Type())
		}
		v.Set(r)
		return nil
	}
	if v.Type() == timeType {
		v.Set(reflect.ValueOf(time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)))
		return nil
	}
	if v.Type() == rawType {
		v.SetBytes([]byte(`{"synthetic":true}`))
		return nil
	}
	*n++
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		return fill(v.Elem(), path, name, overrides, n)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			key, _, keep := field(f)
			if !keep {
				continue
			}
			child := key
			if path != "" {
				child = path + "." + key
			}
			if err := fill(v.Field(i), child, key, overrides, n); err != nil {
				return err
			}
		}
	case reflect.String:
		v.SetString(name)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Cycle within the width so all values remain non-zero even for int8.
		bits := v.Type().Bits()
		maximum := int64(127)
		if bits > 8 {
			maximum = 32767
		}
		v.SetInt(1 + *n%maximum)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		high := uint64(1) << (v.Type().Bits() - 1)
		v.SetUint(high + uint64(*n)%high)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(*n) + 0.375)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		return fill(v.Index(0), path+"[]", name, overrides, n)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := fill(v.Index(i), path+"[]", name, overrides, n); err != nil {
				return err
			}
		}
	case reflect.Map:
		key := reflect.New(v.Type().Key()).Elem()
		item := reflect.New(v.Type().Elem()).Elem()
		if err := fill(key, path+".*key", name, overrides, n); err != nil {
			return err
		}
		if err := fill(item, path+".*", name, overrides, n); err != nil {
			return err
		}
		v.Set(reflect.MakeMap(v.Type()))
		v.SetMapIndex(key, item)
	default:
		return fmt.Errorf("unsupported contract field %s: %s", path, v.Type())
	}
	return nil
}

func field(f reflect.StructField) (string, bool, bool) {
	tag := strings.Split(f.Tag.Get("json"), ",")
	name := tag[0]
	if name == "-" {
		return "", false, false
	}
	if name == "" {
		name = f.Name
	}
	optional := false
	for _, option := range tag[1:] {
		if option == "omitempty" || option == "omitzero" {
			optional = true
		}
	}
	return name, optional, true
}

// Keys records sender required-ness alongside the payload; true means optional.
// This metadata is generated from real types, never a hand-maintained schema.
func Keys(value any) map[string]bool {
	result := map[string]bool{}
	keys(reflect.TypeOf(value), "", result)
	return result
}
func keys(t reflect.Type, path string, out map[string]bool) {
	if t.Kind() == reflect.Pointer {
		keys(t.Elem(), path, out)
		return
	}
	if t == timeType || t == rawType {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			name, optional, keep := field(f)
			if !keep {
				continue
			}
			child := name
			if path != "" {
				child = path + "." + name
			}
			out[child] = optional
			keys(f.Type, child, out)
		}
	case reflect.Slice, reflect.Array:
		keys(t.Elem(), path+"[]", out)
	case reflect.Map:
		keys(t.Elem(), path+".*", out)
	}
}

// Verify checks receiver key presence and whether a sender may omit a key the
// receiver requires. Allowlist entries need a non-empty reason. Decoder checks
// run separately with each receiver's production settings.
func Verify(payload []byte, receiver any, sender map[string]bool, optional map[string]string) error {
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return err
	}
	for path := range Keys(receiver) {
		reason, allowed := optional[path]
		if allowed && strings.TrimSpace(reason) == "" {
			return fmt.Errorf("optional key %s needs a reason", path)
		}
		if !present(document, strings.Split(path, ".")) && !allowed {
			return fmt.Errorf("missing receiver key %s", path)
		}
		mayOmit, exists := sender[path]
		if !exists && !allowed {
			return fmt.Errorf("sender does not declare receiver key %s", path)
		}
		if mayOmit && !allowed {
			return fmt.Errorf("sender may omit required receiver key %s", path)
		}
	}
	return nil
}
func present(value any, parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	part := parts[0]
	if part == "*" {
		m, ok := value.(map[string]any)
		if !ok || len(m) == 0 {
			return false
		}
		for _, item := range m {
			if !present(item, parts[1:]) {
				return false
			}
		}
		return true
	}
	array := strings.HasSuffix(part, "[]")
	key := strings.TrimSuffix(part, "[]")
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	item, ok := m[key]
	if !ok || item == nil {
		return false
	}
	if array {
		a, ok := item.([]any)
		if !ok || len(a) == 0 {
			return false
		}
		for _, element := range a {
			if !present(element, parts[1:]) {
				return false
			}
		}
		return true
	}
	return present(item, parts[1:])
}
