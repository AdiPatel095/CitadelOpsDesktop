package Protocol

import (
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
func TestDecodeKeepsFrameFieldsAndAttachesAView(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	// Same namespace/legacy cases as TestDecodeEmpireExNamespaceAndLegacyResponses.
	for _, tc := range []struct{ raw, namespace, opcode, route, text, payload, payloadText string }{
		{`%xt%EmpireEx%cra%1%{"note":"50% done"}%`, "EmpireEx", "cra", "cra", "1", `{"note":"50% done"}`, ""},
		{`%xt%EmpireEx_21%cra%1%{}%`, "EmpireEx_21", "cra", "cra", "1", `{}`, ""},
		{`%xt%EmpireEx%sin%1%`, "EmpireEx", "sin", "sin", "1", "", ""},
		{`%xt%EmpireEx%ain%1%{}%`, "EmpireEx", "ain", "ain", "1", `{}`, ""},
		{`%xt%cra%1%0%{"MID":123}%`, "", "cra", "1", "0", `{"MID":123}`, ""},
		{`%xt%cra%1%90%{}%`, "", "cra", "1", "90", `{}`, ""},
		{`%xt%EmpireExFoo%1%90%{}%`, "", "empireexfoo", "1", "90", `{}`, ""},
		{`%xt%EmpireEx%cra%bad%not-json%`, "EmpireEx", "cra", "cra", "bad", "", "not-json"},
		{`%xt%gaa%1%0%null%`, "", "gaa", "1", "0", `null`, ""},
		{`%xt%gaa%1%0%[]%`, "", "gaa", "1", "0", `[]`, ""},
	} {
		f, err := Decode(tc.raw, DirectionInbound, at)
		if err != nil {
			t.Fatal(err)
		}
		want := Frame{Direction: DirectionInbound, Transport: "xt", Namespace: tc.namespace, Opcode: tc.opcode, Route: tc.route, ResponseText: tc.text, PayloadText: tc.payloadText, ReceivedAt: at, Raw: tc.raw}
		if tc.payload != "" {
			want.Payload = json.RawMessage(tc.payload)
		}
		if code, err := strconv.Atoi(tc.text); err == nil {
			want.ResponseCode = &code
		}
		if !reflect.DeepEqual(f.WithoutPayloadView(), want) || f.HasPayloadView() != (len(f.Payload) > 0) {
			t.Fatalf("fields=%#v want=%#v", f, want)
		}
		if f.WithoutPayloadView().HasPayloadView() {
			t.Fatal("strip retained view")
		}
	}
}

func TestPayloadRootDecodesOnceAndReturnsPrivateMaps(t *testing.T) {
	f := Frame{Payload: json.RawMessage(`{"a":1,"b":2}`)}.WithPayloadView()
	a, err := f.PayloadRoot()
	if err != nil {
		t.Fatal(err)
	}
	a["new"] = json.RawMessage(`3`)
	delete(a, "a")
	a["b"] = json.RawMessage(`4`)
	b, _ := f.PayloadRoot()
	if string(b["a"]) != "1" || string(b["b"]) != "2" || b["new"] != nil || f.PayloadViewStats().RootDecodes != 1 {
		t.Fatalf("map/stats: %v %v", b, f.PayloadViewStats())
	}
}
func TestPayloadRootMatchesJSONUnmarshal(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[]`, `5`, `""`, ``, `{"a":1,"a":2}`, `{"A":1,"a":2}`} {
		t.Run(raw, func(t *testing.T) {
			var want map[string]json.RawMessage
			err := json.Unmarshal([]byte(raw), &want)
			f := Frame{Payload: json.RawMessage(raw)}.WithPayloadView()
			for range 2 {
				got, e := f.PayloadRoot()
				if !reflect.DeepEqual(got, want) || errorText(e) != errorText(err) {
					t.Fatalf("%v %v != %v %v", got, e, want, err)
				}
			}
			if f.PayloadViewStats().RootDecodes != 1 {
				t.Fatal(f.PayloadViewStats())
			}
		})
	}
}
func referenceRows(raw json.RawMessage) ([][]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var rows [][]json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil, false
	}
	return rows, true
}
func TestPayloadRowsMatchesDecodeRows(t *testing.T) {
	for _, member := range []string{``, `null`, `[]`, `[[1,2,3]]`, `[null]`, `[5]`, `5`, `"x"`, `{}`} {
		t.Run(member, func(t *testing.T) {
			raw := `{}`
			if member != "" {
				raw = `{"AI":` + member + `}`
			}
			f := Frame{Payload: json.RawMessage(raw)}.WithPayloadView()
			want, ok := referenceRows(json.RawMessage(member))
			for range 2 {
				got, valid := f.PayloadRows("AI")
				if valid != ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("rows: %v/%v != %v/%v", got, valid, want, ok)
				}
			}
			if f.PayloadViewStats().RowDecodes != 1 {
				t.Fatal(f.PayloadViewStats())
			}
			_, _ = f.PayloadRows("other")
			if f.PayloadViewStats().RowDecodes != 2 {
				t.Fatal(f.PayloadViewStats())
			}
		})
	}
}
func TestNestedPayloadSharesOneChildView(t *testing.T) {
	f := Frame{Payload: json.RawMessage(`{"gaa":{"KID":0},"nil":null}`)}.WithPayloadView()
	a, found := f.NestedPayload("gaa")
	b, _ := f.NestedPayload("gaa")
	if !found || a.view != b.view {
		t.Fatal("child not shared")
	}
	_, _ = a.PayloadRoot()
	_, _ = b.PayloadRoot()
	if a.PayloadViewStats().RootDecodes != 1 || f.PayloadViewStats().NestedViews != 1 {
		t.Fatal("child counts")
	}
	if _, ok := f.NestedPayload("absent"); ok {
		t.Fatal("absent")
	}
	if n, ok := f.NestedPayload("nil"); !ok || string(n.Payload) != "null" {
		t.Fatal("null")
	}
	if _, ok := (Frame{Payload: json.RawMessage(`5`)}.WithPayloadView()).NestedPayload("a"); ok {
		t.Fatal("root error")
	}
}
func TestReplacedPayloadIsNeverAnsweredFromTheView(t *testing.T) {
	original := Frame{Payload: json.RawMessage(`{"a":1}`)}.WithPayloadView()
	_, _ = original.PayloadRoot()
	replaced := original
	replaced.Payload = json.RawMessage(`{"b":2}`)
	root, err := replaced.PayloadRoot()
	if err != nil || string(root["b"]) != "2" || replaced.HasPayloadView() || replaced.PayloadViewStats() != (PayloadViewStats{}) {
		t.Fatal(root, err)
	}
	if original.PayloadViewStats().RootDecodes != 1 {
		t.Fatal("old changed")
	}
	replaced = replaced.WithPayloadView()
	if replaced.view == original.view {
		t.Fatal("stale view")
	}
	_, _ = replaced.PayloadRoot()
	if replaced.PayloadViewStats().RootDecodes != 1 {
		t.Fatal("new stats")
	}
}
func TestPayloadViewConcurrentUse(t *testing.T) {
	f := Frame{Payload: json.RawMessage(`{"AI":[[1,2,3]],"gaa":{"KID":0}}`)}.WithPayloadView()
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func(frame Frame) {
			defer wg.Done()
			for range 20 {
				r, e := frame.PayloadRoot()
				rows, ok := frame.PayloadRows("AI")
				child, found := frame.NestedPayload("gaa")
				cr, ce := child.PayloadRoot()
				if e != nil || len(r) != 2 || !ok || len(rows) != 1 || !found || ce != nil || string(cr["KID"]) != "0" {
					t.Error("concurrent result")
				}
			}
		}(f)
	}
	wg.Wait()
	if f.PayloadViewStats() != (PayloadViewStats{1, 1, 1}) {
		t.Fatal(f.PayloadViewStats())
	}
}
func TestHasCaseFoldedAlias(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{{`{"KID":0}`, false}, {`{"kid":0}`, true}, {`{"Kid":0}`, true}, {`{"KID":0,"kid":1}`, true}, {`{"KID":0}`, true}, {`{"X":0}`, false}} {
		var root map[string]json.RawMessage
		_ = json.Unmarshal([]byte(tc.raw), &root)
		if got := HasCaseFoldedAlias(root, "KID", "AI"); got != tc.want {
			t.Fatal(tc.raw, got)
		}
	}
}
func TestPlainInt64MatchesExactRationalParse(t *testing.T) {
	rng := rand.New(rand.NewSource(46))
	values := []string{"0", "-0", "1", "-1", "9223372036854775807", "-9223372036854775808", "9223372036854775808", "-9223372036854775809", "01", "-01", "+1", "-", "", "1e3", "1.0", "null", " 1", "1 ", "1/1"}
	for range 10000 {
		v := int64(rng.Uint64())
		raw, _ := json.Marshal(v)
		values = append(values, string(raw))
		if got, ok := PlainInt64(raw); !ok || got != v {
			t.Fatalf("random %d: %d/%v", v, got, ok)
		}
	}
	for _, raw := range values {
		got, ok := PlainInt64([]byte(raw))
		if !ok {
			continue
		}
		r, valid := new(big.Rat).SetString(raw)
		if !json.Valid([]byte(raw)) || !valid || !r.IsInt() || !r.Num().IsInt64() || got != r.Num().Int64() {
			t.Fatalf("%s: %d", raw, got)
		}
	}
	for _, v := range []int64{math.MinInt64, math.MaxInt64, 0, -1} {
		raw, _ := json.Marshal(v)
		if got, ok := PlainInt64(raw); !ok || got != v {
			t.Fatal(v, got, ok)
		}
	}
}

func TestPlainInt64RejectsNonPlainAndOutOfRange(t *testing.T) {
	for _, raw := range []string{"9223372036854775808", "-9223372036854775809", "01", "-01", "+1", "-", "", "1e3", "1.0", "null", " 1", "1 ", "1/1"} {
		if value, ok := PlainInt64([]byte(raw)); ok {
			t.Fatalf("accepted %q: %d", raw, value)
		}
	}
}
