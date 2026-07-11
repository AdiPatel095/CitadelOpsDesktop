package Toolkit

// NewDefaultHarness registers the Citadel runtime adapters while retaining the
// read-only policy unless the embedding process explicitly supplies another one.
func NewDefaultHarness(options ...Option) (*Harness, error) {
	return NewDefaultHarnessWithRuntime(nil, options...)
}

// NewDefaultHarnessWithRuntime exposes the same structured command runtime through the tool
// registry. A companion app can retain the runtime for direct Go calls and expose this Harness
// through JSON, IPC, HTTP, MCP, or another transport without changing command queue semantics.
func NewDefaultHarnessWithRuntime(runtime *CommandRuntime, options ...Option) (*Harness, error) {
	if runtime == nil {
		var err error
		runtime, err = NewCommandRuntime(nil)
		if err != nil {
			return nil, err
		}
	}
	harness := New(options...)
	registrars := []func(*Harness) error{
		registerStateTools,
		registerFeatureTools,
		registerCatalogTools,
		func(h *Harness) error { return registerCommandTools(h, runtime) },
		registerCommandTraceTools,
		func(h *Harness) error { return registerContextCommandTools(h, runtime) },
	}
	for _, register := range registrars {
		if err := register(harness); err != nil {
			return nil, err
		}
	}
	return harness, nil
}
