package Ingest

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type replayEntry struct {
	OffsetMs  int64  `json:"offsetMs"`
	Direction string `json:"direction"`
	Raw       string `json:"raw"`
}
type replayEvent struct {
	Rev        uint64            `json:"rev"`
	Base       uint64            `json:"base"`
	Gap        bool              `json:"gap"`
	Domains    []string          `json:"domains"`
	Components []State.Component `json:"components"`
	Patch      string            `json:"patch"`
}
type replayFrame struct {
	I      int                `json:"i"`
	Op     string             `json:"op"`
	Dir    Protocol.Direction `json:"dir"`
	Rev    uint64             `json:"rev"`
	Err    string             `json:"err"`
	Events []replayEvent      `json:"events"`
}
type replayTrace struct {
	Frames []replayFrame `json:"frames"`
	Final  any           `json:"final"`
}

// Entirely synthetic; generated from documented layouts, never account captures.
func generateReplayCorpus(t *testing.T) {
	t.Helper()
	var entries []replayEntry
	add := func(ms int64, op string, payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, replayEntry{ms, "in", fmt.Sprintf("%%xt%%%s%%1%%0%%%s%%", op, raw)})
	}
	add(0, "gbd", map[string]any{
		"gpi": map[string]any{"PID": 424242, "UID": 777001, "PN": "Fixture Player", "AID": 9001},
		"gcl": map[string]any{"C": []any{map[string]any{"KID": 0, "AI": []any{map[string]any{"AI": []any{1, 100, 100, 100001, 424242, 0, 0, 0, 0, 0, "Fixture Castle 1"}}, map[string]any{"AI": []any{1, 110, 110, 100002, 424242, 0, 0, 0, 0, 0, "Fixture Castle 2"}}}}}},
		"uap": map[string]any{"PMS": 1, "PMT": 300}, "gal": map[string]any{"AID": 9001, "N": "Fixture Alliance"}, "sie": map[string]any{"SP": []any{map[string]any{"STID": 1, "RS": 50, "RSGP": 100}}},
	})
	add(1000, "jaa", map[string]any{"KID": 0, "gca": map[string]any{"A": []any{1, 100, 100, 100001, 424242, 0, 0, 0, 0, 0, "Fixture Castle 1"}, "O": map[string]any{"OID": 424242, "RPT": 600}, "grc": map[string]any{"AID": 100001, "R": []any{[]any{"W", 100}, []any{"F", 200}}}}, "spl": map[string]any{"LID": 0, "PS": map[string]any{"WID": 10, "TUA": 6, "RCT": 75, "PID": 11}, "QS": []any{}}, "sin": []any{map[string]any{"SID": 1, "RD": []any{[]any{10, 20}}}}, "gcu": map[string]any{"C2": 100}})
	for poll := 0; poll < 8; poll++ {
		var movements []any
		for i := 0; i < 4; i++ {
			owner := 424242
			target := 500001
			pt := 1
			if i == 1 {
				pt = 2
			}
			if i == 2 {
				owner = 500002
				target = 424242
			}
			um := map[string]any{"L": map[string]any{"ID": i + 1}}
			if i == 3 {
				um["AAT"] = 1
				um["AAN"] = poll + 1
				um["AAC"] = 12
			}
			movements = append(movements, map[string]any{"M": map[string]any{"MID": 700001 + i, "PT": pt, "TT": 60, "D": 0, "T": 0, "KID": 0, "OID": owner, "TID": target, "SA": []any{1, 100, 100, 100001, owner}, "TA": []any{1, 120 + i, 120, 100010 + i, target}}, "UM": um})
		}
		add(int64(2000+poll*5000), "gam", map[string]any{"M": movements, "O": []any{map[string]any{"OID": 424242, "PRE": 116, "SUF": 31, "TOPX": 50, "CF": 123456}}})
	}
	for window := 0; window < 6; window++ {
		ms := int64(38000 + window*1000)
		base := window * 400
		kingdom := 0
		if window == 3 {
			kingdom = 4
		}
		if window == 4 {
			base = 0
			ms = 38000
		}
		var rows []any
		owners := []any{map[string]any{"OID": 424242, "N": "Fixture Player", "RPT": 600}, map[string]any{"OID": 500001, "N": "Fixture Other"}}
		for i := 0; i < 300; i++ {
			x, y := base+100+i, 100
			var row []any
			switch i % 10 {
			case 0:
				row = []any{1, x, y, 100001 + i, 424242, 0, 0, 0, 0, 0, fmt.Sprintf("Fixture Castle %d", i+3)}
			case 1:
				row = []any{2, x, y, -1, 845, 100, 0}
			case 2:
				row = []any{11, x, y, -1, 45, 60, 0, kingdom}
			case 3:
				row = []any{21, x, y, 70, -1, 0}
			case 4:
				row = []any{34, x, y, 80, -1, 1}
			case 5:
				row = []any{24, x, y, 100010 + i, -1, 4, "Fixture Isle", 0, 1, 60}
			case 6:
				row = []any{25, x, y, 4, 0, 1, 60, 4, 0}
			case 7:
				row = []any{35, x, y, -1, 1, 60, 0, 0, 5001, 0, 0, 0, 0}
			case 8:
				typ := 29
				if i%20 == 18 {
					typ = 30
				}
				row = []any{typ, x, y, -1, 9, 60, 0, 0, 5001, 11, 22, 33}
			case 9:
				row = []any{999, x, y, -1}
			}
			if window == 5 && i == 0 {
				row = []any{999, 100, 100, -1}
			}
			rows = append(rows, row)
		}
		req := fmt.Sprintf("%%xt%%EmpireEx_21%%gaa%%1%%{\"KID\":%d,\"AX1\":%d,\"AY1\":100,\"AX2\":%d,\"AY2\":100}%%", kingdom, base+100, base+399)
		entries = append(entries, replayEntry{ms, "out", req})
		add(ms, "gaa", map[string]any{"KID": kingdom, "AI": rows, "OI": owners})
	}
	for i, op := range []string{"fnm", "fnt", "ssi"} {
		kid := 0
		if op == "fnt" {
			kid = 10
		}
		add(int64(44000+i*10), op, map[string]any{"X": 100, "Y": 100, "gaa": map[string]any{"KID": kid, "AI": []any{[]any{2, 100, 100, -1, 1, 60, 0}}}})
	}
	add(44100, "ain", map[string]any{"A": map[string]any{"AID": 9001, "N": "Fixture Alliance", "M": []any{map[string]any{"OID": 424242, "N": "Fixture Player", "RPT": 600, "AP": []any{[]any{0, 100001, 100, 100, 1}, []any{0, 100002, 110, 110, 1}}}}}})
	add(44110, "jca", map[string]any{"KID": 0, "gca": map[string]any{"A": []any{1, 110, 110, 100002, 424242}, "O": map[string]any{"OID": 424242, "RPT": 600}}})
	for i, item := range []struct{ op, raw string }{{"gaa", `{"KID":0,"AI":5}`}, {"gam", `{"M":"x"}`}, {"gam", `{"M":[],"O":"x"}`}, {"jaa", `{}`}, {"gaa", `{"kid":0,"ai":[[2,100,100,-1,1,0,0]]}`}} {
		entries = append(entries, replayEntry{int64(44200 + i), "in", fmt.Sprintf("%%xt%%%s%%1%%0%%%s%%", item.op, item.raw)})
	}
	entries = append(entries, replayEntry{44300, "in", "%xt%gaa%1%1%{}%"})
	var output bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		output.Write(b)
		output.WriteByte('\n')
	}
	if err := os.WriteFile("testdata/ingest_session_replay.jsonl", output.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func normalizeReplay(t *testing.T, value any, start time.Time) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if root, ok := result.(map[string]any); ok {
		delete(root, "updatedAt")
		delete(root, "occurredAt")
	}
	var walk func(any) any
	walk = func(v any) any {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				v[k] = walk(x)
			}
		case []any:
			for i, x := range v {
				v[i] = walk(x)
			}
		case string:
			if at, e := time.Parse(time.RFC3339Nano, v); e == nil && at.Sub(start) >= -30*24*time.Hour && at.Sub(start) <= 30*24*time.Hour {
				return fmt.Sprintf("T+%dms", at.Sub(start).Milliseconds())
			}
		}
		return v
	}
	return walk(result)
}

func runReplay(t *testing.T, start time.Time) replayTrace {
	t.Helper()
	raw, err := os.ReadFile("testdata/ingest_session_gamedata.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Session = State.SessionState{LoggedIn: true, SocketReady: true, Generation: 1, ConnectionGeneration: 1, ChangedAt: start.Add(-time.Second)}
	store := State.NewStore(&state)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, staticGameDataProvider{data}, registry)
	events, unsubscribe := store.Subscribe(4096)
	defer unsubscribe()
	input, err := os.Open("testdata/ingest_session_replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	trace := replayTrace{}
	for scanner.Scan() {
		var entry replayEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		direction := Protocol.DirectionInbound
		if entry.Direction == "out" {
			direction = Protocol.DirectionOutbound
		}
		committed, err := pipeline.HandleRawAt(t.Context(), entry.Raw, direction, start.Add(time.Duration(entry.OffsetMs)*time.Millisecond))
		f := replayFrame{I: len(trace.Frames), Op: committed.Frame.Opcode, Dir: direction, Rev: committed.Revision, Events: []replayEvent{}}
		if err != nil {
			f.Err = err.Error()
		}
	drain:
		for {
			select {
			case e := <-events:
				patch, _ := json.Marshal(normalizeReplay(t, e.Patch, start))
				f.Events = append(f.Events, replayEvent{e.Revision, e.BaseRevision, e.Gap, e.Domains, e.Components, fmt.Sprintf("%x", sha256.Sum256(patch))})
			default:
				break drain
			}
		}
		trace.Frames = append(trace.Frames, f)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	accessorState1 := store.ReadOnlyView()
	trace.Final = normalizeReplay(t, State.NewClientStateSnapshot(&accessorState1), start)
	return trace
}

// Return the first precise JSON path, including frame and event array indices.
func replayDifference(a, b any, path string) string {
	if reflect.DeepEqual(a, b) {
		return ""
	}
	switch a := a.(type) {
	case map[string]any:
		bb, ok := b.(map[string]any)
		if !ok {
			return path
		}
		keys := make([]string, 0, len(a))
		for k := range a {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if d := replayDifference(a[k], bb[k], path+"."+k); d != "" {
				return d
			}
		}
		for k := range bb {
			if _, ok := a[k]; !ok {
				return path + "." + k
			}
		}
	case []any:
		bb, ok := b.([]any)
		if !ok || len(a) != len(bb) {
			return path + ".length"
		}
		for i := range a {
			if d := replayDifference(a[i], bb[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
	}
	return path
}
func TestIngestReplayCorpusMatchesGolden(t *testing.T) {
	if os.Getenv("CITADEL_REGENERATE_INGEST_CORPUS") == "1" {
		generateReplayCorpus(t)
	}
	start := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	first := runReplay(t, start)
	time.Sleep(20 * time.Millisecond)
	second := runReplay(t, start.Add(-time.Hour-37*time.Minute-13*time.Second))
	actual, _ := json.MarshalIndent(first, "", "  ")
	other, _ := json.MarshalIndent(second, "", "  ")
	actual = append(actual, '\n')
	other = append(other, '\n')
	compare := func(want, got []byte, label string) {
		t.Helper()
		if bytes.Equal(want, got) {
			return
		}
		path := filepath.Join(t.TempDir(), "actual.json")
		_ = os.WriteFile(path, got, 0644)
		var a, b any
		_ = json.Unmarshal(want, &a)
		_ = json.Unmarshal(got, &b)
		t.Fatalf("%s differs at %s; actual: %s", label, replayDifference(a, b, "$"), path)
	}
	compare(actual, other, "determinism pre-check")
	golden := "testdata/ingest_session_replay.golden.json"
	if os.Getenv("CITADEL_UPDATE_INGEST_GOLDEN") == "1" {
		if err := os.WriteFile(golden, actual, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	compare(want, actual, "golden")
}
