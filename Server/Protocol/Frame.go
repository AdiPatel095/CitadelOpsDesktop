package Protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type Frame struct {
	Direction            Direction       `json:"direction"`
	Transport            string          `json:"transport"`
	Namespace            string          `json:"namespace,omitempty"`
	Opcode               string          `json:"opcode"`
	Route                string          `json:"route,omitempty"`
	ResponseCode         *int            `json:"responseCode,omitempty"`
	ResponseText         string          `json:"responseText,omitempty"`
	Payload              json.RawMessage `json:"payload,omitempty"`
	PayloadText          string          `json:"payloadText,omitempty"`
	ReceivedAt           time.Time       `json:"receivedAt"`
	Raw                  string          `json:"-"`
	ResponseToken        string          `json:"-"`
	CausationOperationID string          `json:"causationOperationId,omitempty"`
}

func Decode(raw string, direction Direction, receivedAt time.Time) (Frame, error) {
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	parts := strings.SplitN(raw, "%", 6)
	if len(parts) != 6 || parts[0] != "" {
		return Frame{}, fmt.Errorf("invalid xt frame header")
	}
	payload := strings.TrimSuffix(parts[5], "%")
	frame := Frame{
		Direction:    direction,
		Transport:    strings.TrimSpace(parts[1]),
		Route:        strings.TrimSpace(parts[3]),
		ResponseText: strings.TrimSpace(parts[4]),
		ReceivedAt:   receivedAt,
		Raw:          raw,
	}
	if frame.Transport == "" {
		return Frame{}, fmt.Errorf("xt frame transport is empty")
	}
	if strings.HasPrefix(parts[2], "EmpireEx_") {
		frame.Namespace = strings.TrimSpace(parts[2])
		frame.Opcode = strings.ToLower(strings.TrimSpace(parts[3]))
	} else {
		frame.Opcode = strings.ToLower(strings.TrimSpace(parts[2]))
	}
	if frame.Opcode == "" {
		return Frame{}, fmt.Errorf("xt frame opcode is empty")
	}
	if responseCode, err := strconv.Atoi(frame.ResponseText); err == nil {
		frame.ResponseCode = &responseCode
	}
	if payload != "" {
		if json.Valid([]byte(payload)) {
			frame.Payload = json.RawMessage(payload)
		} else {
			frame.PayloadText = payload
		}
	}
	return frame, nil
}

type Command struct {
	Namespace     string          `json:"namespace,omitempty"`
	Opcode        string          `json:"opcode"`
	Sequence      string          `json:"sequence,omitempty"`
	Route         string          `json:"route,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	Bare          bool            `json:"bare,omitempty"`
	OmitNamespace bool            `json:"omitNamespace,omitempty"`
}

func Encode(command Command) ([]byte, error) {
	namespace := strings.TrimSpace(command.Namespace)
	if command.OmitNamespace {
		namespace = ""
	}
	opcode := strings.ToLower(strings.TrimSpace(command.Opcode))
	sequence := strings.TrimSpace(command.Sequence)
	route := strings.TrimSpace(command.Route)
	if sequence == "" {
		sequence = "1"
	}
	for name, value := range map[string]string{"namespace": namespace, "opcode": opcode, "sequence": sequence, "route": route} {
		if strings.Contains(value, "%") {
			return nil, fmt.Errorf("command %s contains a delimiter", name)
		}
	}
	if opcode == "" {
		return nil, fmt.Errorf("command opcode is required")
	}
	if command.Bare {
		if namespace == "" {
			return []byte(fmt.Sprintf("%%xt%%%s%%%s%%", opcode, sequence)), nil
		}
		return []byte(fmt.Sprintf("%%xt%%%s%%%s%%%s%%", namespace, opcode, sequence)), nil
	}
	payload := command.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("command payload is not valid JSON")
	}
	if namespace == "" {
		if route != "" {
			return []byte(fmt.Sprintf("%%xt%%%s%%%s%%%s%%%s%%", opcode, sequence, route, payload)), nil
		}
		return []byte(fmt.Sprintf("%%xt%%%s%%%s%%%s%%", opcode, sequence, payload)), nil
	}
	return []byte(fmt.Sprintf("%%xt%%%s%%%s%%%s%%%s%%", namespace, opcode, sequence, payload)), nil
}

type CommittedFrame struct {
	Frame       Frame    `json:"frame"`
	IngressID   uint64   `json:"ingressId,omitempty"`
	Revision    uint64   `json:"revision"`
	Domains     []string `json:"domains,omitempty"`
	ReduceError string   `json:"-"`
}
