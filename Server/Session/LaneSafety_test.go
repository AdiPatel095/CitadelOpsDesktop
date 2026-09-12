package Session

import (
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestRequestOpcodeRejectionCorrelatesDespiteSuccessAlias(t *testing.T) {
	newPending := func(token string) directPendingResponse {
		return directPendingResponse{token: token, requestOpcode: "jca", opcodes: map[string]struct{}{"jaa": {}}, expiresAt: time.Now().Add(time.Minute)}
	}
	transport := &DirectWebSocketTransport{pending: []directPendingResponse{newPending("operation/1")}}
	code := 0
	frame := Protocol.Frame{Opcode: "jca", ResponseCode: &code}
	if token := transport.matchResponseToken(frame); token != "" || len(transport.pending) != 1 {
		t.Fatal("acknowledgement consumed aliased response wait")
	}
	code = 311
	if token := transport.matchResponseToken(frame); token != "operation/1" || len(transport.pending) != 0 {
		t.Fatalf("rejection token=%q", token)
	}
	transport.pending = []directPendingResponse{newPending("one"), newPending("two")}
	if token := transport.matchResponseToken(frame); token != "" || len(transport.pending) != 2 {
		t.Fatal("ambiguous rejection attributed to arbitrary lane")
	}
}
