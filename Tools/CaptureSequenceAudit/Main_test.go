package main

import "testing"

func TestParseCaptureLineUsesNamespacedWireOpcode(t *testing.T) {
	line := "2026-01-02 03:04:05.123456 [SEND] [EmpireEx_21] %xt%EmpireEx_21%bup%1%{\"AID\":42}%"

	_, direction, opcode, ok := parseCaptureLine(line)
	if !ok {
		t.Fatal("expected capture line to parse")
	}
	if direction != "SEND" || opcode != "bup" {
		t.Fatalf("parsed direction/opcode = %q/%q, want SEND/bup", direction, opcode)
	}
}

func TestParseCaptureLineUsesTokenlessWireOpcode(t *testing.T) {
	line := "2026-01-02 03:04:05.123456 [SEND] [aec] %xt%aec%1%0%[]%"

	_, direction, opcode, ok := parseCaptureLine(line)
	if !ok {
		t.Fatal("expected capture line to parse")
	}
	if direction != "SEND" || opcode != "aec" {
		t.Fatalf("parsed direction/opcode = %q/%q, want SEND/aec", direction, opcode)
	}
}

func TestCaptureResponseCode(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "tokenless",
			line: "2026-01-02 03:04:05.123456 [RECV] [cra] %xt%cra%1%0%{}%",
			want: "0",
		},
		{
			name: "namespaced",
			line: "2026-01-02 03:04:05.123456 [RECV] [bup] %xt%EmpireEx_21%bup%1%63%{}%",
			want: "63",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := captureResponseCode(test.line)
			if !ok || got != test.want {
				t.Fatalf("captureResponseCode() = %q, %v, want %q, true", got, ok, test.want)
			}
		})
	}
}
