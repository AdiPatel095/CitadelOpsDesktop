package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	protocoltest "CitadelDesktop/Server/ProtocolTestHarness"
)

const maxLogLineBytes = 16 * 1024 * 1024

var safeOpcodes = map[string]bool{
	"ain":  true,
	"boi":  true,
	"bsd":  true,
	"bup":  true,
	"cmi":  true,
	"crin": true,
	"csm":  true,
	"dcl":  true,
	"gam":  true,
	"gdi":  true,
	"gei":  true,
	"ggm":  true,
	"hru":  true,
	"jaa":  true,
	"kpi":  true,
	"seq":  true,
	"sge":  true,
}

var appSendLinePattern = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}\.\d+) \[(SEND|MATCH)\] \[([^]]+)\] (.*)$`,
)

type extractionOptions struct {
	Generation string
	Source     string
	Location   *time.Location
	MaxDelay   time.Duration
	Opcodes    map[string]bool
}

type appSendRecord struct {
	Timestamp time.Time
	Direction string
	Opcode    string
	Frame     string
	Line      int
}

type extractedManifest struct {
	SchemaVersion int                                `json:"schemaVersion"`
	LiveCommands  []protocoltest.LiveCommandContract `json:"liveCommands"`
}

func main() {
	input := flag.String("input", "Logs/channels/app_send.log", "app_send log to stream")
	source := flag.String("source", "", "repository-relative source path stored in evidence (defaults to input)")
	generation := flag.String("generation", "", "required capture generation label, for example live-app/2026-07")
	opcodesText := flag.String("opcodes", defaultOpcodeList(), "comma-separated reviewed opcode allowlist")
	locationName := flag.String("timezone", "America/New_York", "IANA timezone used by log timestamps")
	maxDelay := flag.Duration("max-delay", 5*time.Second, "maximum SEND-to-MATCH pairing delay")
	flag.Parse()

	location, err := time.LoadLocation(*locationName)
	if err != nil {
		fail("load timezone: %v", err)
	}
	opcodes, err := parseOpcodes(*opcodesText)
	if err != nil {
		fail("opcodes: %v", err)
	}
	storedSource, err := evidenceSource(*input, *source)
	if err != nil {
		fail("source: %v", err)
	}
	file, err := os.Open(*input)
	if err != nil {
		fail("open input: %v", err)
	}
	defer file.Close()

	contracts, err := extractLiveCommands(file, extractionOptions{
		Generation: strings.TrimSpace(*generation),
		Source:     storedSource,
		Location:   location,
		MaxDelay:   *maxDelay,
		Opcodes:    opcodes,
	})
	if err != nil {
		fail("extract: %v", err)
	}
	output := extractedManifest{SchemaVersion: protocoltest.SchemaVersion, LiveCommands: contracts}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		fail("encode output: %v", err)
	}
}

func extractLiveCommands(reader io.Reader, options extractionOptions) ([]protocoltest.LiveCommandContract, error) {
	if strings.TrimSpace(options.Generation) == "" {
		return nil, fmt.Errorf("generation is required")
	}
	if strings.TrimSpace(options.Source) == "" {
		return nil, fmt.Errorf("source is required")
	}
	if options.Location == nil {
		return nil, fmt.Errorf("location is required")
	}
	if options.MaxDelay <= 0 {
		return nil, fmt.Errorf("max delay must be positive")
	}
	if len(options.Opcodes) == 0 {
		return nil, fmt.Errorf("at least one opcode is required")
	}

	pending := make(map[string][]appSendRecord, len(options.Opcodes))
	latest := make(map[string]protocoltest.LiveCommandContract, len(options.Opcodes))
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		record, ok, err := parseAppSendLine(scanner.Text(), lineNumber, options.Location)
		if err != nil {
			return nil, err
		}
		if !ok || !options.Opcodes[record.Opcode] {
			continue
		}
		switch record.Direction {
		case "SEND":
			frame, err := protocoltest.ParseFrame(record.Frame)
			if err != nil || !frame.HasBody || frame.Opcode != record.Opcode {
				continue
			}
			pending[record.Opcode] = append(pending[record.Opcode], record)
		case "MATCH":
			responseOpcode, status, err := parseResponseEnvelope(record.Frame)
			if err != nil || responseOpcode != record.Opcode {
				continue
			}
			queue := pending[record.Opcode]
			for len(queue) > 0 && record.Timestamp.Sub(queue[0].Timestamp) > options.MaxDelay {
				queue = queue[1:]
			}
			if len(queue) == 0 {
				pending[record.Opcode] = nil
				continue
			}
			request := queue[0]
			pending[record.Opcode] = queue[1:]
			delay := record.Timestamp.Sub(request.Timestamp)
			if delay < 0 || delay > options.MaxDelay || status != 0 {
				continue
			}
			digest := sha256.Sum256([]byte(record.Frame))
			latest[record.Opcode] = protocoltest.LiveCommandContract{
				Name:               fmt.Sprintf("live_%s_%s", request.Timestamp.Format("2006_01_02"), record.Opcode),
				Generation:         options.Generation,
				Adapter:            record.Opcode,
				CapturedAt:         request.Timestamp.Format(time.RFC3339Nano),
				Source:             options.Source,
				SourceRequestLine:  request.Line,
				SourceResponseLine: record.Line,
				RequestFrame:       request.Frame,
				Response: protocoltest.LiveResponseEvidence{
					Opcode:      responseOpcode,
					Status:      status,
					DelayMS:     delay.Milliseconds(),
					FrameBytes:  len([]byte(record.Frame)),
					FrameSHA256: hex.EncodeToString(digest[:]),
				},
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(latest) == 0 {
		return nil, fmt.Errorf("no successful allowlisted SEND/MATCH pairs found")
	}
	opcodes := make([]string, 0, len(latest))
	for opcode := range latest {
		opcodes = append(opcodes, opcode)
	}
	sort.Strings(opcodes)
	contracts := make([]protocoltest.LiveCommandContract, 0, len(opcodes))
	for _, opcode := range opcodes {
		contracts = append(contracts, latest[opcode])
	}
	manifest := protocoltest.Manifest{SchemaVersion: protocoltest.SchemaVersion, LiveCommands: contracts}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validate extracted contracts: %w", err)
	}
	return contracts, nil
}

func parseAppSendLine(line string, lineNumber int, location *time.Location) (appSendRecord, bool, error) {
	match := appSendLinePattern.FindStringSubmatch(line)
	if len(match) == 0 {
		return appSendRecord{}, false, nil
	}
	timestamp, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", match[1]+" "+match[2], location)
	if err != nil {
		return appSendRecord{}, false, fmt.Errorf("line %d timestamp: %w", lineNumber, err)
	}
	return appSendRecord{
		Timestamp: timestamp,
		Direction: match[3],
		Opcode:    strings.ToLower(strings.TrimSpace(match[4])),
		Frame:     strings.TrimSpace(match[5]),
		Line:      lineNumber,
	}, true, nil
}

func parseResponseEnvelope(frame string) (string, int, error) {
	if !strings.HasPrefix(frame, "%xt%") {
		return "", 0, fmt.Errorf("response is missing %%xt%% prefix")
	}
	rest := strings.TrimPrefix(frame, "%xt%")
	first, rest, ok := strings.Cut(rest, "%")
	if !ok || first == "" {
		return "", 0, fmt.Errorf("response is missing opcode")
	}
	opcode := first
	if strings.HasPrefix(first, "EmpireEx_") {
		opcode, rest, ok = strings.Cut(rest, "%")
		if !ok || opcode == "" {
			return "", 0, fmt.Errorf("EmpireEx response is missing opcode")
		}
	}
	_, rest, ok = strings.Cut(rest, "%")
	if !ok {
		return "", 0, fmt.Errorf("response is missing sequence")
	}
	statusText, _, ok := strings.Cut(rest, "%")
	if !ok {
		return "", 0, fmt.Errorf("response is missing status")
	}
	status, err := strconv.Atoi(statusText)
	if err != nil {
		return "", 0, fmt.Errorf("response status %q: %w", statusText, err)
	}
	return strings.ToLower(opcode), status, nil
}

func parseOpcodes(value string) (map[string]bool, error) {
	opcodes := make(map[string]bool)
	for _, raw := range strings.Split(value, ",") {
		opcode := strings.ToLower(strings.TrimSpace(raw))
		if opcode == "" {
			continue
		}
		if !safeOpcodes[opcode] {
			return nil, fmt.Errorf("opcode %q is not in the reviewed safe allowlist", opcode)
		}
		opcodes[opcode] = true
	}
	if len(opcodes) == 0 {
		return nil, errors.New("no opcodes selected")
	}
	return opcodes, nil
}

func defaultOpcodeList() string {
	opcodes := make([]string, 0, len(safeOpcodes))
	for opcode := range safeOpcodes {
		opcodes = append(opcodes, opcode)
	}
	sort.Strings(opcodes)
	return strings.Join(opcodes, ",")
}

func evidenceSource(input, source string) (string, error) {
	if source = strings.TrimSpace(source); source != "" {
		return filepath.ToSlash(filepath.Clean(source)), nil
	}
	input = filepath.Clean(input)
	if !filepath.IsAbs(input) {
		return filepath.ToSlash(input), nil
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(workingDirectory, input)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("absolute input is outside the working directory; pass -source explicitly")
	}
	return filepath.ToSlash(relative), nil
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
