package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const captureTimestampLayout = "2006-01-02 15:04:05.000000"

type captureEvent struct {
	at     time.Time
	opcode string
}

type targetAudit struct {
	count         int
	printed       int
	responseCodes map[string]int
}

func main() {
	directory := flag.String("dir", "Logs/channels/websocket_game", "directory containing websocket capture logs")
	targetList := flag.String("targets", "cra", "comma-separated outbound opcodes to audit")
	lookback := flag.Duration("lookback", 15*time.Second, "outbound context window before each target")
	limit := flag.Int("limit", 20, "maximum detailed occurrences printed per target; zero prints only totals")
	previous := flag.Int("previous", 12, "maximum number of prior outbound commands printed per occurrence")
	flag.Parse()

	targets := parseTargets(*targetList)
	files, err := filepath.Glob(filepath.Join(*directory, "*.log"))
	if err != nil {
		fail(err)
	}
	sort.Strings(files)
	audits := make(map[string]*targetAudit, len(targets))
	for target := range targets {
		audits[target] = &targetAudit{responseCodes: map[string]int{}}
	}
	for _, path := range files {
		if err := auditFile(path, targets, audits, *lookback, *limit, *previous); err != nil {
			fail(err)
		}
	}
	keys := make([]string, 0, len(audits))
	for target := range audits {
		keys = append(keys, target)
	}
	sort.Strings(keys)
	fmt.Println("TOTALS")
	for _, target := range keys {
		fmt.Printf("%s\toutbound=%d\tresponses=%s\n", target, audits[target].count, formatResponseCodes(audits[target].responseCodes))
	}
}

func auditFile(
	path string,
	targets map[string]struct{},
	audits map[string]*targetAudit,
	lookback time.Duration,
	limit int,
	previous int,
) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	history := make([]captureEvent, 0, 32)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		at, direction, opcode, ok := parseCaptureLine(line)
		if !ok {
			continue
		}
		if direction == "RECV" {
			if _, wanted := targets[opcode]; wanted {
				if code, found := captureResponseCode(line); found {
					audits[opcode].responseCodes[code]++
				}
			}
			continue
		}
		if direction != "SEND" {
			continue
		}
		minimum := at.Add(-lookback)
		first := 0
		for first < len(history) && history[first].at.Before(minimum) {
			first++
		}
		history = append(history[:0], history[first:]...)
		if _, wanted := targets[opcode]; wanted {
			audit := audits[opcode]
			audit.count++
			if limit > 0 && audit.printed < limit {
				fmt.Printf("%s:%d\t%s\t%s\n", filepath.Base(path), lineNumber, opcode, formatHistory(history, at, previous))
				audit.printed++
			}
		}
		history = append(history, captureEvent{at: at, opcode: opcode})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

func parseCaptureLine(line string) (time.Time, string, string, bool) {
	if len(line) < len(captureTimestampLayout) {
		return time.Time{}, "", "", false
	}
	at, err := time.ParseInLocation(captureTimestampLayout, line[:len(captureTimestampLayout)], time.Local)
	if err != nil {
		return time.Time{}, "", "", false
	}
	directionStart := strings.Index(line, " [")
	if directionStart < 0 {
		return time.Time{}, "", "", false
	}
	directionStart += 2
	directionEnd := strings.Index(line[directionStart:], "]")
	if directionEnd < 0 {
		return time.Time{}, "", "", false
	}
	directionEnd += directionStart
	opcodeStart := strings.Index(line[directionEnd+1:], "[")
	if opcodeStart < 0 {
		return time.Time{}, "", "", false
	}
	opcodeStart += directionEnd + 2
	opcodeEnd := strings.Index(line[opcodeStart:], "]")
	if opcodeEnd < 0 {
		return time.Time{}, "", "", false
	}
	opcodeEnd += opcodeStart
	direction := line[directionStart:directionEnd]
	opcode := strings.ToLower(strings.TrimSpace(line[opcodeStart:opcodeEnd]))
	if direction == "SEND" {
		wireParts := strings.SplitN(line[opcodeEnd+1:], "%", 6)
		if len(wireParts) >= 4 && wireParts[1] == "xt" {
			wireOpcode := wireParts[2]
			if strings.HasPrefix(wireOpcode, "EmpireEx_") && len(wireParts) >= 5 {
				wireOpcode = wireParts[3]
			}
			opcode = strings.ToLower(strings.TrimSpace(wireOpcode))
		}
	}
	return at, direction, opcode, true
}

func formatHistory(history []captureEvent, targetAt time.Time, maximum int) string {
	if len(history) == 0 {
		return "(no prior outbound command in window)"
	}
	if maximum > 0 && len(history) > maximum {
		history = history[len(history)-maximum:]
	}
	parts := make([]string, 0, len(history))
	for _, event := range history {
		parts = append(parts, fmt.Sprintf("%s@-%s", event.opcode, targetAt.Sub(event.at).Round(time.Millisecond)))
	}
	return strings.Join(parts, " ")
}

func parseTargets(value string) map[string]struct{} {
	targets := map[string]struct{}{}
	for _, target := range strings.Split(value, ",") {
		target = strings.ToLower(strings.TrimSpace(target))
		if target != "" {
			targets[target] = struct{}{}
		}
	}
	return targets
}

func captureResponseCode(line string) (string, bool) {
	start := strings.Index(line, "%xt%")
	if start < 0 {
		return "", false
	}
	parts := strings.SplitN(line[start:], "%", 7)
	if len(parts) < 5 || parts[1] != "xt" {
		return "", false
	}
	if strings.HasPrefix(parts[2], "EmpireEx_") {
		if len(parts) < 6 {
			return "", false
		}
		return parts[5], true
	}
	return parts[4], true
}

func formatResponseCodes(codes map[string]int) string {
	if len(codes) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(codes))
	for code := range codes {
		keys = append(keys, code)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, code := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", code, codes[code]))
	}
	return strings.Join(parts, ",")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
