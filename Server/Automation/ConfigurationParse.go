package Automation

import (
	"sync"
	"sync/atomic"
)

// Configuration sections are parsed once per distinct content, not once per
// evaluation (CIT-43). Configuration stores publish immutable snapshots and a
// new revision changes the section bytes, so keying by content gives "once per
// revision per runtime" and lets runtimes with identical settings share a parse.
// Cached values are shared and read-only.

const sectionMemoLimit = 128

type sectionMemo[T any] struct {
	mu      sync.Mutex
	entries map[string]T
}

func (memo *sectionMemo[T]) get(raw []byte, build func() T) T {
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if value, found := memo.entries[string(raw)]; found {
		return value
	}
	value := build()
	if memo.entries == nil || len(memo.entries) >= sectionMemoLimit {
		memo.entries = make(map[string]T, 8)
	}
	memo.entries[string(raw)] = value
	return value
}

// configurationParses counts real parses of the enabled-controls and scheduler
// sections; tests use it to prove evaluations do not reparse.
var configurationParses atomic.Int64
