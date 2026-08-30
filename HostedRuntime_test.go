package main

import "testing"

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
