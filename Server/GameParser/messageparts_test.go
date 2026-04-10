package GameParser

import "testing"

func TestCommandType(t *testing.T) {
	_, ok := CommandType([]string{"a", "b"})
	if ok {
		t.Fatal("expected false for short slice")
	}
	cmd, ok := CommandType([]string{"0", "1", "jaa"})
	if !ok || cmd != "jaa" {
		t.Fatalf("got %q ok=%v", cmd, ok)
	}
}

func TestPayload(t *testing.T) {
	_, ok := Payload([]string{"0", "1", "2", "3", "4"})
	if ok {
		t.Fatal("expected false when len<=5 (no index 5)")
	}
	parts := []string{"0", "1", "cmd", "3", "4", "json-here", "tail"}
	p, ok := Payload(parts)
	if !ok || p != "json-here" {
		t.Fatalf("got %q ok=%v", p, ok)
	}
}
