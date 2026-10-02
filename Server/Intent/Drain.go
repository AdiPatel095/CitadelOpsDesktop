package Intent

import (
	"context"
	"errors"
	"time"
)

// FinishOpen closes admission while existing mutating operations finish.
// Reads are cancelled and recorded as failed; no operation is replayed.
// Success leaves admission closed until the owner stops or explicitly resumes
// the runtime. Any failed wait automatically restores admission.
func (engine *Engine) FinishOpen(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine.admissionMu.Lock()
	if engine.draining {
		engine.admissionMu.Unlock()
		return errors.New("runtime is already draining")
	}
	engine.draining = true
	engine.admissionMu.Unlock()
	finished := false
	defer func() {
		if !finished {
			engine.ResumeAdmission()
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		engine.mu.Lock()
		for id, cancel := range engine.active {
			receipt := engine.operations[id]
			if receipt.Plan != nil && receipt.Plan.Effect == EffectRead {
				if engine.drainCancelled == nil {
					engine.drainCancelled = map[string]bool{}
				}
				engine.drainCancelled[id] = true
				cancel()
			}
		}
		idle := engine.inflight == 0
		engine.mu.Unlock()
		if idle {
			finished = true
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (engine *Engine) ResumeAdmission() {
	engine.admissionMu.Lock()
	engine.draining = false
	engine.admissionMu.Unlock()
}
