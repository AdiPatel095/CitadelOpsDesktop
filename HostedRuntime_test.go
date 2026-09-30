package main

import (
	"fmt"
	"strings"
	"testing"

	"CitadelDesktop/Server/Profiling"
)

func TestDesktopRemainsDefaultNOneComposition(t *testing.T) {
	if hostedModeEnabled(false, "") {
		t.Fatal("desktop startup unexpectedly selected hosted mode")
	}
	if !hostedModeEnabled(true, "") || !hostedModeEnabled(false, "/tmp/tenant.json") {
		t.Fatal("explicit hosted composition was not selected")
	}
}

func TestCloudPortChangesOnlyHostedImplicitListenAddress(t *testing.T) {
	if got := defaultListenAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("desktop listen address = %q", got)
	}
	if got := resolvedListenAddress(defaultListenAddress(), false, false, "9090"); got != "127.0.0.1:8080" {
		t.Fatalf("desktop cloud-port address = %q", got)
	}
	if got := resolvedListenAddress(defaultListenAddress(), true, false, "9090"); got != "0.0.0.0:9090" {
		t.Fatalf("hosted cloud-port address = %q", got)
	}
	if got := resolvedListenAddress("127.0.0.1:7777", true, true, "9090"); got != "127.0.0.1:7777" {
		t.Fatalf("explicit hosted address = %q", got)
	}
}

func TestHostedProfilingIsOffWithoutAnAddress(t *testing.T) {
	var logged []string
	stop := startHostedProfiling(t.Context(), "", func(format string, args ...any) { logged = append(logged, format) })
	defer stop()
	if Profiling.Enabled() || len(logged) != 0 {
		t.Fatalf("profiling enabled or logged without an address: %v", logged)
	}
}

func TestHostedProfilingRefusesNonLoopbackAndKeepsRunning(t *testing.T) {
	for _, address := range []string{"0.0.0.0:6060", ":6060", "[::]:6060", "10.0.0.7:6060", "citadelops.app:6060"} {
		var logged string
		stop := startHostedProfiling(t.Context(), address, func(format string, args ...any) {
			logged = fmt.Sprintf(format, args...)
		})
		stop()
		if Profiling.Enabled() {
			t.Fatalf("profiling enabled for %q", address)
		}
		if !strings.Contains(logged, "not started") || !strings.Contains(logged, Profiling.EnvAddr) {
			t.Fatalf("refusal of %q was not logged with its reason: %q", address, logged)
		}
	}
}

func TestHostedProfilingListensOnLoopbackOnly(t *testing.T) {
	stop := startHostedProfiling(t.Context(), "127.0.0.1:0", t.Logf)
	defer stop()
	if !Profiling.Enabled() {
		t.Fatal("profiling is not enabled for a loopback address")
	}
	stop()
	if Profiling.Enabled() {
		t.Fatal("profiling stays enabled after stop")
	}
}
