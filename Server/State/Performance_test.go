package State

import "testing"

func BenchmarkStoreSnapshotCurrentData(benchmark *testing.B) {
	state, err := LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_ = store.Snapshot()
	}
}

func BenchmarkStorePlanningViewCurrentData(benchmark *testing.B) {
	state, err := LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_ = store.PlanningView()
	}
}

func BenchmarkStoreApplyCurrentData(benchmark *testing.B) {
	state, err := LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
			state.Session.Generation++
			return []string{"session"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyWithoutMapMutationCurrentData(benchmark *testing.B) {
	state, err := LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyWithoutMapMutation(func(state *GameState) ([]string, bool, error) {
			state.Session.Generation++
			return []string{"session"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}
