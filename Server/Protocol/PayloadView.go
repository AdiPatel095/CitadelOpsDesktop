package Protocol

import (
	"encoding/json"
	"maps"
	"strings"
	"sync"
)

type payloadView struct {
	payload json.RawMessage
	mu      sync.Mutex
	rootSet bool
	root    map[string]json.RawMessage
	rootErr error
	rows    map[string]payloadRows
	nested  map[string]*payloadView
	stats   PayloadViewStats
}
type payloadRows struct {
	rows [][]json.RawMessage
	ok   bool
}

// PayloadViewStats counts decodes performed by one view, for diagnostics and tests.
type PayloadViewStats struct {
	RootDecodes int
	RowDecodes  int
	NestedViews int
}

func newPayloadView(payload json.RawMessage) *payloadView { return &payloadView{payload: payload} }
func (v *payloadView) describes(payload json.RawMessage) bool {
	return len(v.payload) == len(payload) && (len(payload) == 0 || &v.payload[0] == &payload[0])
}
func (frame Frame) HasPayloadView() bool {
	return frame.view != nil && frame.view.describes(frame.Payload)
}
func (frame Frame) WithPayloadView() Frame {
	if !frame.HasPayloadView() {
		frame.view = newPayloadView(frame.Payload)
	}
	return frame
}
func (frame Frame) WithoutPayloadView() Frame { frame.view = nil; return frame }
func (frame Frame) PayloadViewStats() PayloadViewStats {
	if !frame.HasPayloadView() {
		return PayloadViewStats{}
	}
	frame.view.mu.Lock()
	defer frame.view.mu.Unlock()
	return frame.view.stats
}
func (v *payloadView) decodeRootLocked() {
	if !v.rootSet {
		v.rootErr = json.Unmarshal(v.payload, &v.root)
		v.rootSet = true
		v.stats.RootDecodes++
	}
}

// PayloadRoot returns a private map; RawMessage bytes are shared and read-only.
func (frame Frame) PayloadRoot() (map[string]json.RawMessage, error) {
	if !frame.HasPayloadView() {
		var root map[string]json.RawMessage
		err := json.Unmarshal(frame.Payload, &root)
		return root, err
	}
	v := frame.view
	v.mu.Lock()
	defer v.mu.Unlock()
	v.decodeRootLocked()
	return maps.Clone(v.root), v.rootErr
}
func decodePayloadRows(root map[string]json.RawMessage, err error, key string) ([][]json.RawMessage, bool) {
	if err != nil || len(root[key]) == 0 {
		return nil, false
	}
	var rows [][]json.RawMessage
	if json.Unmarshal(root[key], &rows) != nil {
		return nil, false
	}
	return rows, true
}

// PayloadRows returns shared read-only rows, memoized per key.
func (frame Frame) PayloadRows(key string) ([][]json.RawMessage, bool) {
	if !frame.HasPayloadView() {
		root, err := frame.PayloadRoot()
		return decodePayloadRows(root, err, key)
	}
	v := frame.view
	v.mu.Lock()
	defer v.mu.Unlock()
	v.decodeRootLocked()
	if result, found := v.rows[key]; found {
		return result.rows, result.ok
	}
	rows, ok := decodePayloadRows(v.root, v.rootErr, key)
	if v.rows == nil {
		v.rows = make(map[string]payloadRows)
	}
	v.rows[key] = payloadRows{rows, ok}
	v.stats.RowDecodes++
	return rows, ok
}
func (frame Frame) NestedPayload(key string) (Frame, bool) {
	if !frame.HasPayloadView() {
		root, err := frame.PayloadRoot()
		raw, found := root[key]
		if err != nil || !found {
			return Frame{}, false
		}
		frame.Payload = raw
		frame.view = nil
		return frame, true
	}
	v := frame.view
	v.mu.Lock()
	defer v.mu.Unlock()
	v.decodeRootLocked()
	raw, found := v.root[key]
	if v.rootErr != nil || !found {
		return Frame{}, false
	}
	child := v.nested[key]
	if child == nil {
		child = newPayloadView(raw)
		if v.nested == nil {
			v.nested = make(map[string]*payloadView)
		}
		v.nested[key] = child
		v.stats.NestedViews++
	}
	frame.Payload = raw
	frame.view = child
	return frame, true
}
func HasCaseFoldedAlias(root map[string]json.RawMessage, names ...string) bool {
	for key := range root {
		for _, name := range names {
			if key != name && strings.EqualFold(key, name) {
				return true
			}
		}
	}
	return false
}
