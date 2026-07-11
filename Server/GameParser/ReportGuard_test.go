package GameParser

import (
	"errors"
	"testing"
)

func TestReportResponsePayloadRejectsMissingReportStatus(t *testing.T) {
	if _, err := reportResponsePayload([]string{"", "xt", "bsd", "1", "130", ""}); !errors.Is(err, errReportUnavailable) {
		t.Fatalf("reportResponsePayload() error = %v, want report unavailable", err)
	}
}

func TestReportResponsePayloadAcceptsJSON(t *testing.T) {
	payload, err := reportResponsePayload([]string{"", "xt", "bsd", "1", "0", `{}`})
	if err != nil {
		t.Fatalf("reportResponsePayload() error = %v", err)
	}
	if payload != `{}` {
		t.Fatalf("reportResponsePayload() = %q, want {}", payload)
	}
}
