package Telemetry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"CitadelDesktop/Server/Protocol"
)

type Channel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Store struct {
	mu       sync.RWMutex
	capacity int
	lines    map[string][]string
}

func NewStore(capacity int) *Store {
	if capacity < 100 {
		capacity = 100
	}
	return &Store{capacity: capacity, lines: map[string][]string{}}
}

func (store *Store) Record(frame Protocol.CommittedFrame, reduceErr error) {
	direction := strings.ToUpper(string(frame.Frame.Direction))
	if frame.Frame.Direction == Protocol.DirectionInbound {
		direction = "RECV"
	} else if frame.Frame.Direction == Protocol.DirectionOutbound {
		direction = "SEND"
	}
	payload := strings.TrimSpace(string(frame.Frame.Payload))
	if sensitiveOpcode(frame.Frame.Opcode) && payload != "" {
		payload = "[redacted]"
	}
	if len(payload) > 4096 {
		payload = payload[:4096] + "…"
	}
	line := fmt.Sprintf("%s [%s] [%s] %s", frame.Frame.ReceivedAt.Format("2006-01-02 15:04:05.000000"), direction, frame.Frame.Opcode, payload)
	store.append("protocol", line)
	if frame.Frame.Direction == Protocol.DirectionOutbound {
		store.append("commands", line)
	}
	if reduceErr != nil {
		store.append("errors", line+" error="+reduceErr.Error())
	}
}

func (store *Store) Channels() []Channel {
	store.mu.RLock()
	defer store.mu.RUnlock()
	labels := map[string]string{"protocol": "Protocol", "commands": "Commands", "errors": "Errors"}
	channels := make([]Channel, 0, len(store.lines))
	for id := range store.lines {
		channels = append(channels, Channel{ID: id, Label: labels[id]})
	}
	sort.Slice(channels, func(left, right int) bool { return channels[left].ID < channels[right].ID })
	return channels
}

func (store *Store) Tail(channel string, limit int) []string {
	if limit < 1 {
		limit = 100
	}
	if limit > store.capacity {
		limit = store.capacity
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	lines := store.lines[channel]
	start := max(0, len(lines)-limit)
	return append([]string(nil), lines[start:]...)
}

func (store *Store) append(channel string, line string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	lines := append(store.lines[channel], line)
	if len(lines) > store.capacity {
		copy(lines, lines[len(lines)-store.capacity:])
		lines = lines[:store.capacity]
	}
	store.lines[channel] = lines
}

func sensitiveOpcode(opcode string) bool {
	switch strings.ToLower(opcode) {
	case "lli", "lfe", "rlu", "vck":
		return true
	default:
		return false
	}
}
