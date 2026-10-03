package Intent

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// DispatchEvidenceCollector must return an allowlisted, identifier-free value.
// It runs inside the router's final validation, after every dispatch guard.
type DispatchEvidenceCollector func(context.Context, PlanningContext, Step, string, []byte) any

func (engine *Engine) SetDispatchEvidenceCollector(collector DispatchEvidenceCollector) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.dispatchEvidenceCollector = collector
}

type dispatchResolverContextKey struct{}
type dispatchBoundaryContextKey struct{}
type dispatchBoundaryBuffer struct {
	mu       sync.Mutex
	snapshot any
}

func (buffer *dispatchBoundaryBuffer) set(value any) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.snapshot = value
}
func dispatchEvidenceOpcode(opcode string) bool {
	switch strings.ToLower(opcode) {
	case "hru", "ahr", "fco", "crm":
		return true
	}
	return false
}
func recordDispatchRejection(ctx context.Context, err error) {
	var response *ResponseCodeError
	if !errors.As(err, &response) {
		return
	}
	opcode := strings.ToLower(response.Opcode)
	if !((opcode == "hru" && response.Meaning.Code == 63) ||
		(opcode == "ahr" && response.Meaning.Code == 2) ||
		(opcode == "fco" && response.Meaning.Code == 5) ||
		(opcode == "crm" && response.Meaning.Code == 109)) {
		return
	}
	buffer, _ := ctx.Value(dispatchBoundaryContextKey{}).(*dispatchBoundaryBuffer)
	if buffer == nil {
		return
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.snapshot != nil {
		_ = RecordOperationEvidence(ctx, "dispatch_boundary", buffer.snapshot)
		buffer.snapshot = nil
	}
}
