package Ingest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type Pipeline struct {
	state    *State.Store
	gameData GameDataProvider
	registry *Registry

	watchMu   sync.RWMutex
	watchers  map[uint64]frameWatcher
	nextWatch atomic.Uint64
	telemetry frameTelemetry
}

type frameTelemetry interface {
	Record(frame Protocol.CommittedFrame, reduceErr error)
}

type frameWatcher struct {
	opcode        string
	afterRevision uint64
	channel       chan Protocol.CommittedFrame
}

func NewPipeline(state *State.Store, gameData GameDataProvider, registry *Registry) *Pipeline {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Pipeline{
		state: state, gameData: gameData, registry: registry,
		watchers: map[uint64]frameWatcher{},
	}
}

func (pipeline *Pipeline) Registry() *Registry {
	return pipeline.registry
}

func (pipeline *Pipeline) SetTelemetry(telemetry frameTelemetry) {
	pipeline.telemetry = telemetry
}

func (pipeline *Pipeline) HandleRaw(ctx context.Context, raw string, direction Protocol.Direction) (Protocol.CommittedFrame, error) {
	frame, err := Protocol.Decode(raw, direction, time.Now().UTC())
	if err != nil {
		return Protocol.CommittedFrame{}, err
	}
	return pipeline.HandleFrame(ctx, frame)
}

func (pipeline *Pipeline) HandleFrame(ctx context.Context, frame Protocol.Frame) (Protocol.CommittedFrame, error) {
	if pipeline.state == nil {
		return Protocol.CommittedFrame{}, fmt.Errorf("ingest state store is unavailable")
	}
	var currentData *GameData.Store
	if pipeline.gameData != nil {
		currentData, _ = pipeline.gameData.Current()
	}
	reducer := pipeline.registry.reducer(frame.Opcode)
	event, err := pipeline.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		recordObservation(gameState, frame, "")
		domains := []string{"protocol"}
		if reducer != nil && frame.Direction == Protocol.DirectionInbound {
			reducerDomains, reducerChanged, reduceErr := reducer(ctx, frame, gameState, currentData)
			if reduceErr != nil {
				return nil, false, fmt.Errorf("reduce %s: %w", frame.Opcode, reduceErr)
			}
			if reducerChanged {
				domains = append(domains, reducerDomains...)
			}
		}
		return domains, true, nil
	})
	if err != nil {
		reduceErr := err
		event, err = pipeline.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
			recordObservation(gameState, frame, reduceErr.Error())
			return []string{"protocol"}, true, nil
		})
		if err != nil {
			return Protocol.CommittedFrame{}, fmt.Errorf("record failed %s frame: %w", frame.Opcode, err)
		}
		committed := Protocol.CommittedFrame{Frame: frame, Revision: event.Revision, Domains: event.Domains}
		pipeline.publish(committed)
		if pipeline.telemetry != nil {
			pipeline.telemetry.Record(committed, reduceErr)
		}
		return committed, reduceErr
	}
	committed := Protocol.CommittedFrame{Frame: frame, Revision: event.Revision, Domains: event.Domains}
	pipeline.publish(committed)
	if pipeline.telemetry != nil {
		pipeline.telemetry.Record(committed, nil)
	}
	return committed, nil
}

func (pipeline *Pipeline) Watch(opcode string, afterRevision uint64) (<-chan Protocol.CommittedFrame, func()) {
	id := pipeline.nextWatch.Add(1)
	channel := make(chan Protocol.CommittedFrame, 1)
	pipeline.watchMu.Lock()
	pipeline.watchers[id] = frameWatcher{opcode: strings.ToLower(strings.TrimSpace(opcode)), afterRevision: afterRevision, channel: channel}
	pipeline.watchMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			pipeline.watchMu.Lock()
			delete(pipeline.watchers, id)
			pipeline.watchMu.Unlock()
		})
	}
	return channel, cancel
}

func recordObservation(gameState *State.GameState, frame Protocol.Frame, lastError string) {
	observation := gameState.Observations[frame.Opcode]
	observation.Opcode = frame.Opcode
	observation.Count++
	if frame.Direction == Protocol.DirectionInbound {
		observation.InboundCount++
	} else if frame.Direction == Protocol.DirectionOutbound {
		observation.OutboundCount++
	}
	observation.LastDirection = string(frame.Direction)
	observation.LastCode = frame.ResponseCode
	observation.LastError = lastError
	observation.LastSeenAt = frame.ReceivedAt
	observation.LastRevision = gameState.Revision + 1
	gameState.Observations[frame.Opcode] = observation
}

func (pipeline *Pipeline) publish(frame Protocol.CommittedFrame) {
	pipeline.watchMu.RLock()
	defer pipeline.watchMu.RUnlock()
	for _, watcher := range pipeline.watchers {
		if frame.Frame.Direction != Protocol.DirectionInbound {
			continue
		}
		if watcher.opcode != "" && watcher.opcode != frame.Frame.Opcode {
			continue
		}
		if frame.Revision <= watcher.afterRevision {
			continue
		}
		select {
		case watcher.channel <- frame:
		default:
		}
	}
}
