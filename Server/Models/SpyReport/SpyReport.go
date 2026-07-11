package spyreport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	battlereport "CitadelDesktop/Server/Models/BattleReport"
	reportnotice "CitadelDesktop/Server/Models/ReportNotice"
	"CitadelDesktop/Server/Paths"
)

const maxScanLineSize = 32 * 1024 * 1024

type Notice struct {
	MID       int64         `json:"mid"`
	BattleKey string        `json:"battleKey,omitempty"`
	SNERow    []interface{} `json:"sneRow,omitempty"`
	AutoShare bool          `json:"-"`
}

type Capture struct {
	Version              int                    `json:"version"`
	MID                  int64                  `json:"mid"`
	BattleKey            string                 `json:"battleKey,omitempty"`
	CapturedAtUnixMillis int64                  `json:"capturedAtUnixMillis"`
	SNERow               []interface{}          `json:"sneRow,omitempty"`
	BSD                  map[string]interface{} `json:"bsd,omitempty"`
}

type Player struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Alliance string `json:"alliance,omitempty"`
}

type Castle struct {
	ID        int64  `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	KingdomID int    `json:"kingdomID,omitempty"`
	X         int    `json:"x,omitempty"`
	Y         int    `json:"y,omitempty"`
}

type UnitCount struct {
	WodID  int   `json:"wodID"`
	Amount int64 `json:"amount"`
}

type SetupSection struct {
	Index int         `json:"index"`
	Name  string      `json:"name"`
	Units []UnitCount `json:"units"`
	Total int64       `json:"total"`
}

type Castellan struct {
	Level             int                   `json:"level,omitempty"`
	Wall              int                   `json:"wall,omitempty"`
	Gate              int                   `json:"gate,omitempty"`
	Moat              int                   `json:"moat,omitempty"`
	Courtyard         int                   `json:"courtyard,omitempty"`
	WallSlots         int                   `json:"wallSlots,omitempty"`
	Effects           []interface{}         `json:"effects,omitempty"`
	Equipment         []interface{}         `json:"equipment,omitempty"`
	SkillIDs          []interface{}         `json:"skillIDs,omitempty"`
	CalculatedEffects []battlereport.Effect `json:"calculatedEffects,omitempty"`
}

type ParsedReport struct {
	ID                   string         `json:"id"`
	MID                  int64          `json:"mid"`
	CapturedAtUnixMillis int64          `json:"capturedAtUnixMillis"`
	Status               string         `json:"status"`
	Accuracy             int            `json:"accuracy,omitempty"`
	Risk                 int            `json:"risk,omitempty"`
	SpyCount             int            `json:"spyCount,omitempty"`
	GuardCount           int            `json:"guardCount,omitempty"`
	Target               Player         `json:"target"`
	Source               Player         `json:"source"`
	Castle               Castle         `json:"castle"`
	Setup                []SetupSection `json:"setup,omitempty"`
	TotalTroops          int64          `json:"totalTroops,omitempty"`
	Castellan            *Castellan     `json:"castellan,omitempty"`
	RawCapture           *Capture       `json:"rawCapture,omitempty"`
}

type CastleNameIndex struct {
	ByCastleID   map[int64]string
	ByCoordinate map[string]string
}

var archiveMu sync.Mutex

var castleNameCache struct {
	Path            string
	Size            int64
	ModifiedAtNanos int64
	Index           CastleNameIndex
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/spy-reports", handleListReports)
	mux.HandleFunc("GET /api/spyReports", handleListReports)
}

func NoticesFromSNEPayload(payload string) ([]Notice, error) {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	var root map[string]interface{}
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	rows, _ := root["MSG"].([]interface{})
	notices := make([]Notice, 0)
	for _, raw := range rows {
		row, ok := raw.([]interface{})
		if !ok || len(row) < 2 || intFrom(row[1]) != 3 || !reportnotice.IsSpyFetchableRow(row) {
			continue
		}
		mid := int64From(row[0])
		if mid <= 0 {
			continue
		}
		notice := Notice{MID: mid, SNERow: row}
		if len(row) > 2 {
			notice.BattleKey, _ = row[2].(string)
		}
		notices = append(notices, notice)
	}
	return notices, nil
}

func UpsertCapture(capture Capture) error {
	if capture.MID <= 0 {
		return fmt.Errorf("spy report MID is required")
	}
	if capture.Version == 0 {
		capture.Version = 1
	}
	if capture.CapturedAtUnixMillis == 0 {
		capture.CapturedAtUnixMillis = time.Now().UnixMilli()
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	captures, err := readCaptures()
	if err != nil {
		return err
	}
	replaced := false
	for i := range captures {
		if captures[i].MID == capture.MID {
			captures[i] = capture
			replaced = true
			break
		}
	}
	if !replaced {
		captures = append(captures, capture)
	}
	return writeCaptures(captures)
}

func ParseCapture(capture Capture) ParsedReport {
	report := ParsedReport{ID: strconv.FormatInt(capture.MID, 10), MID: capture.MID, CapturedAtUnixMillis: capture.CapturedAtUnixMillis, Status: "failed"}
	bsd := capture.BSD
	if bsd == nil {
		return report
	}
	report.Accuracy = intFrom(bsd["SA"])
	report.Risk = intFrom(bsd["SR"])
	report.SpyCount = intFrom(bsd["SC"])
	report.GuardCount = intFrom(bsd["GC"])
	report.Target = parsePlayer(bsd["OI"])
	report.Source = parsePlayer(bsd["SO"])
	report.Castle = parseCastle(bsd)
	report.Setup = parseSetup(bsd["S"])
	for _, section := range report.Setup {
		report.TotalTroops += section.Total
	}
	if block, ok := bsd["B"].(map[string]interface{}); ok && len(block) > 0 {
		report.Castellan = parseCastellan(block)
	}
	if len(report.Setup) > 0 && report.Castellan != nil {
		report.Status = "success"
	} else if len(report.Setup) > 0 || report.Castellan != nil {
		report.Status = "partial"
	}
	return report
}

// IsPlayerCastleTarget reports whether BSD identifies a real player-owned castle.
// DUM is true for dummy/NPC owners; requiring explicit false avoids forwarding ambiguous targets.
func IsPlayerCastleTarget(capture Capture) bool {
	if capture.BSD == nil || int64From(capture.BSD["CID"]) <= 0 {
		return false
	}
	owner, ok := capture.BSD["OI"].(map[string]interface{})
	if !ok || int64From(owner["OID"]) <= 0 {
		return false
	}
	dummy, present := owner["DUM"].(bool)
	return present && !dummy
}

func ReadParsedReports() ([]ParsedReport, error) {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	captures, err := readCaptures()
	if err != nil {
		return nil, err
	}
	reports := make([]ParsedReport, 0, len(captures))
	for _, capture := range captures {
		reports = append(reports, ParseCapture(capture))
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].CapturedAtUnixMillis > reports[j].CapturedAtUnixMillis })
	return reports, nil
}

// ReadCastleNameIndex reads only the fields needed by target lists. It avoids
// calculating troop and castellan details for every archived spy report.
func ReadCastleNameIndex() (CastleNameIndex, error) {
	archiveMu.Lock()
	defer archiveMu.Unlock()

	path := archivePath()
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return CastleNameIndex{}, err
	}
	if err == nil && castleNameCache.Path == path && castleNameCache.Size == info.Size() && castleNameCache.ModifiedAtNanos == info.ModTime().UnixNano() {
		return cloneCastleNameIndex(castleNameCache.Index), nil
	}

	index := CastleNameIndex{
		ByCastleID:   make(map[int64]string),
		ByCoordinate: make(map[string]string),
	}
	captures, readErr := readCaptures()
	if readErr != nil {
		return CastleNameIndex{}, readErr
	}
	for _, capture := range captures {
		if capture.BSD == nil {
			continue
		}
		ai, _ := capture.BSD["AI"].(map[string]interface{})
		name := strings.TrimSpace(stringFrom(ai["N"]))
		if name == "" {
			continue
		}
		if castleID := int64From(capture.BSD["CID"]); castleID > 0 {
			index.ByCastleID[castleID] = name
		}
		index.ByCoordinate[fmt.Sprintf("%d_%d", intFrom(ai["X"]), intFrom(ai["Y"]))] = name
	}
	if info != nil {
		castleNameCache.Path = path
		castleNameCache.Size = info.Size()
		castleNameCache.ModifiedAtNanos = info.ModTime().UnixNano()
		castleNameCache.Index = cloneCastleNameIndex(index)
	}
	return index, nil
}

func cloneCastleNameIndex(index CastleNameIndex) CastleNameIndex {
	clone := CastleNameIndex{
		ByCastleID:   make(map[int64]string, len(index.ByCastleID)),
		ByCoordinate: make(map[string]string, len(index.ByCoordinate)),
	}
	for castleID, name := range index.ByCastleID {
		clone.ByCastleID[castleID] = name
	}
	for coordinate, name := range index.ByCoordinate {
		clone.ByCoordinate[coordinate] = name
	}
	return clone
}

func ReadCaptures() ([]Capture, error) {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	return readCaptures()
}

func handleListReports(w http.ResponseWriter, _ *http.Request) {
	reports, err := ReadParsedReports()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cloudReports, cloudErr := FetchAllianceSpyReports(); cloudErr == nil {
		reports = mergeParsedReports(reports, cloudReports)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reports)
}

func mergeParsedReports(local, cloud []ParsedReport) []ParsedReport {
	byID := make(map[string]ParsedReport, len(local)+len(cloud))
	for _, report := range cloud {
		report.RawCapture = nil
		byID[report.ID] = report
	}
	for _, report := range local {
		byID[report.ID] = report
	}
	merged := make([]ParsedReport, 0, len(byID))
	for _, report := range byID {
		merged = append(merged, report)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].CapturedAtUnixMillis > merged[j].CapturedAtUnixMillis })
	return merged
}

func parseSetup(raw interface{}) []SetupSection {
	rows, _ := raw.([]interface{})
	names := []string{"Left flank", "Center", "Right flank", "Courtyard", "Reserve I", "Reserve II", "Reserve III"}
	sections := make([]SetupSection, 0, len(rows))
	for index, rawSection := range rows {
		unitsRaw, _ := rawSection.([]interface{})
		section := SetupSection{Index: index, Name: fmt.Sprintf("Section %d", index+1), Units: []UnitCount{}}
		if index < len(names) {
			section.Name = names[index]
		}
		for _, rawUnit := range unitsRaw {
			pair, _ := rawUnit.([]interface{})
			if len(pair) < 2 {
				continue
			}
			unit := UnitCount{WodID: intFrom(pair[0]), Amount: int64From(pair[1])}
			if unit.WodID <= 0 || unit.Amount <= 0 {
				continue
			}
			section.Units = append(section.Units, unit)
			section.Total += unit.Amount
		}
		if len(section.Units) > 0 {
			sections = append(sections, section)
		}
	}
	return sections
}

func parsePlayer(raw interface{}) Player {
	data, _ := raw.(map[string]interface{})
	return Player{ID: int64From(data["OID"]), Name: stringFrom(data["N"]), Alliance: stringFrom(data["AN"])}
}

func parseCastle(bsd map[string]interface{}) Castle {
	ai, _ := bsd["AI"].(map[string]interface{})
	return Castle{ID: int64From(bsd["CID"]), Name: stringFrom(ai["N"]), KingdomID: intFrom(ai["K"]), X: intFrom(ai["X"]), Y: intFrom(ai["Y"])}
}

func parseCastellan(data map[string]interface{}) *Castellan {
	return &Castellan{Level: intFrom(data["L"]), Wall: intFrom(data["W"]), Gate: intFrom(data["GID"]), Moat: intFrom(data["D"]), Courtyard: intFrom(data["SPR"]), WallSlots: intFrom(data["ST"]), Effects: sliceFrom(data["AE"]), Equipment: sliceFrom(data["EQ"]), SkillIDs: sliceFrom(data["SIDS"]), CalculatedEffects: battlereport.ParsePlayerCastellanEffects(data)}
}

func readCaptures() ([]Capture, error) {
	f, err := os.Open(archivePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Capture{}, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxScanLineSize)
	captures := make([]Capture, 0)
	for scanner.Scan() {
		var capture Capture
		if json.Unmarshal(scanner.Bytes(), &capture) == nil && capture.MID > 0 {
			captures = append(captures, capture)
		}
	}
	return captures, scanner.Err()
}

func writeCaptures(captures []Capture) error {
	path := archivePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f)
	for _, capture := range captures {
		if err := encoder.Encode(capture); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func archivePath() string                       { return filepath.Join(Paths.DataDir(), "SpyReports.jsonl") }
func sliceFrom(value interface{}) []interface{} { result, _ := value.([]interface{}); return result }
func stringFrom(value interface{}) string       { result, _ := value.(string); return result }
func intFrom(value interface{}) int             { return int(int64From(value)) }
func int64From(value interface{}) int64 {
	switch typed := value.(type) {
	case json.Number:
		result, _ := typed.Int64()
		return result
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	}
	return 0
}
