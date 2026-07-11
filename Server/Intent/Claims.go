package Intent

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type claimManager struct {
	mu     sync.Mutex
	claims map[string]chan struct{}
}

func newClaimManager() *claimManager {
	return &claimManager{claims: map[string]chan struct{}{}}
}

func (manager *claimManager) acquire(ctx context.Context, requested []string) (func(), error) {
	claims := normalizeClaims(requested)
	acquired := make([]chan struct{}, 0, len(claims))
	for _, claim := range claims {
		manager.mu.Lock()
		semaphore := manager.claims[claim]
		if semaphore == nil {
			semaphore = make(chan struct{}, 1)
			semaphore <- struct{}{}
			manager.claims[claim] = semaphore
		}
		manager.mu.Unlock()
		select {
		case <-ctx.Done():
			for index := len(acquired) - 1; index >= 0; index-- {
				acquired[index] <- struct{}{}
			}
			return nil, ctx.Err()
		case <-semaphore:
			acquired = append(acquired, semaphore)
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(acquired) - 1; index >= 0; index-- {
				acquired[index] <- struct{}{}
			}
		})
	}, nil
}

func normalizeClaims(claims []string) []string {
	set := map[string]struct{}{}
	for _, claim := range claims {
		claim = strings.TrimSpace(claim)
		if claim != "" {
			set[claim] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for claim := range set {
		out = append(out, claim)
	}
	sort.Strings(out)
	return out
}
